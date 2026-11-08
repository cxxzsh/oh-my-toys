package traffic

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/cxxzsh/oh-my-toys/internal/platform/trust"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/tools/traffic/ca"
)

// makeRoot makes the root certificate the tool would make.
func makeRoot(t *testing.T, tl *Tool) *ca.Authority {
	t.Helper()
	root, err := ca.Create(tl.caDir)
	if err != nil {
		t.Fatalf("ca.Create: %v", err)
	}
	return root
}

func TestCertWithoutARoot(t *testing.T) {
	tl, fake := testTool(t)
	state, err := tl.Cert()
	if err != nil {
		t.Fatalf("Cert: %v", err)
	}
	if state != (tool.CertState{}) {
		t.Errorf("Cert = %+v, want the zero state", state)
	}
	if fake.seen("trusted") {
		t.Error("the trust store was asked about a certificate that does not exist")
	}
}

func TestCertStates(t *testing.T) {
	tl, fake := testTool(t)
	root := makeRoot(t, tl)

	fake.trusted = true
	state, err := tl.Cert()
	if err != nil {
		t.Fatalf("Cert: %v", err)
	}
	if !state.Exists || !state.Trusted {
		t.Errorf("Cert = %+v, want an existing, trusted certificate", state)
	}
	if state.Fingerprint != root.Fingerprint() {
		t.Errorf("fingerprint %q, want %q", state.Fingerprint, root.Fingerprint())
	}
	if state.Path != root.CertPath() {
		t.Errorf("path %q, want %q", state.Path, root.CertPath())
	}
	if _, err := os.Stat(state.Path); err != nil {
		t.Errorf("stat %s: %v", state.Path, err)
	}
	if state.NotAfter.IsZero() {
		t.Error("NotAfter is zero")
	}

	fake.trusted = false
	if state, err = tl.Cert(); err != nil || state.Trusted {
		t.Errorf("Cert = %+v, %v, want an existing, untrusted certificate", state, err)
	}

	fake.trustErr = trust.ErrPermission
	state, err = tl.Cert()
	if !errors.Is(err, tool.ErrPermission) {
		t.Errorf("Cert: %v, want tool.ErrPermission", err)
	}
	if !state.Exists || state.Trusted {
		t.Errorf("Cert = %+v, want Exists with Trusted false", state)
	}
}

func TestInstallCertMakesTheRootAndTrustsIt(t *testing.T) {
	tl, fake := testTool(t)
	if err := tl.InstallCert(context.Background()); err != nil {
		t.Fatalf("InstallCert: %v", err)
	}
	if len(fake.installed) != 1 {
		t.Fatalf("%d certificates installed, want one", len(fake.installed))
	}
	root, err := ca.Load(tl.caDir)
	if err != nil {
		t.Fatalf("ca.Load: %v", err)
	}
	if fake.installed[0] != string(root.CertPEM()) {
		t.Error("the certificate installed is not the root of the tool")
	}
}

func TestInstallCertReportsARefusedPrompt(t *testing.T) {
	tl, fake := testTool(t)
	fake.trustErr = trust.ErrPermission
	err := tl.InstallCert(context.Background())
	if !errors.Is(err, tool.ErrPermission) {
		t.Errorf("InstallCert: %v, want tool.ErrPermission", err)
	}
	if _, loadErr := ca.Load(tl.caDir); loadErr != nil {
		t.Errorf("ca.Load: %v, want the root kept for the next try", loadErr)
	}
}

func TestInstallCertKeepsAnExistingRoot(t *testing.T) {
	tl, fake := testTool(t)
	root := makeRoot(t, tl)
	if err := tl.InstallCert(context.Background()); err != nil {
		t.Fatalf("InstallCert: %v", err)
	}
	again, err := ca.Load(tl.caDir)
	if err != nil {
		t.Fatalf("ca.Load: %v", err)
	}
	if again.Fingerprint() != root.Fingerprint() {
		t.Error("a second install replaced the root")
	}
	if len(fake.installed) != 1 {
		t.Errorf("%d installations, want one", len(fake.installed))
	}
}

func TestRemoveCertRemovesTheRootAndStopsDecrypting(t *testing.T) {
	tl, fake := testTool(t)
	root := makeRoot(t, tl)
	cfg := proxyConfig(t, false)
	cfg.Decrypt = true
	if err := tl.SetConfig(cfg); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if err := tl.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = tl.Stop() })
	if st := tl.Status(); st.Detail == "" {
		t.Fatal("the capture does not report decrypting, so the test proves nothing")
	}

	if err := tl.RemoveCert(context.Background()); err != nil {
		t.Fatalf("RemoveCert: %v", err)
	}
	if len(fake.untrusted) != 1 || fake.untrusted[0] != string(root.CertPEM()) {
		t.Errorf("untrusted %d certificates, want the root", len(fake.untrusted))
	}
	if _, err := ca.Load(tl.caDir); !errors.Is(err, ca.ErrNotFound) {
		t.Errorf("ca.Load: %v, want the root gone", err)
	}
	for _, name := range []string{"root.pem", "root.key"} {
		if _, err := os.Stat(filepath.Join(tl.caDir, name)); !os.IsNotExist(err) {
			t.Errorf("stat %s: %v, want it gone", name, err)
		}
	}
	run, ok := tl.runningProxy()
	if !ok {
		t.Fatal("the capture stopped instead of restarting")
	}
	if run.decrypting {
		t.Error("the capture still opens HTTPS with the removed root")
	}
	if st := tl.Status(); st.Detail != "" {
		t.Errorf("status detail %q, want none: HTTPS is tunnelled now", st.Detail)
	}
	if _, err := ca.Load(tl.caDir); !errors.Is(err, ca.ErrNotFound) {
		t.Error("a restarting capture made a new root behind the person's back")
	}
}

func TestRemoveCertKeepsTheRootWhenTheTrustStoreRefuses(t *testing.T) {
	tl, fake := testTool(t)
	root := makeRoot(t, tl)
	fake.untrustErr = trust.ErrUnsupported
	err := tl.RemoveCert(context.Background())
	if !errors.Is(err, tool.ErrUnsupportedHere) {
		t.Errorf("RemoveCert: %v, want tool.ErrUnsupportedHere", err)
	}
	again, loadErr := ca.Load(tl.caDir)
	if loadErr != nil {
		t.Fatalf("ca.Load: %v, want the root kept: without its PEM the app can neither report nor remove it", loadErr)
	}
	if again.Fingerprint() != root.Fingerprint() {
		t.Error("the root changed although the trust store refused")
	}
}

func TestRemoveCertWithoutARootDoesNothing(t *testing.T) {
	tl, fake := testTool(t)
	if err := tl.RemoveCert(context.Background()); err != nil {
		t.Fatalf("RemoveCert: %v", err)
	}
	if fake.seen("trust remove") {
		t.Error("the trust store was changed although no root exists")
	}
}
