package trust

import (
	"bytes"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// liveEnv turns on the tests that change the real system trust store.
const liveEnv = "OMT_LIVE"

// systemBundle is the bundle update-ca-certificates rebuilds from the
// anchor directory. The test reads it directly, not through the package.
const systemBundle = "/etc/ssl/certs/ca-certificates.crt"

// requireLiveRoot gates a live test. Without OMT_LIVE it skips; with it,
// a missing prerequisite fails, so CI cannot go green without running.
func requireLiveRoot(t *testing.T) {
	t.Helper()
	if os.Getenv(liveEnv) == "" {
		t.Skip("set OMT_LIVE=1 to change the real system")
	}
	if os.Geteuid() != 0 {
		t.Fatalf("live trust test must run as root (use sudo): it changes the system trust store")
	}
	for _, p := range []string{debianUpdate, debianAnchors, systemBundle} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("live trust test needs a Debian-like system: %v", err)
		}
	}
}

// certDER returns the DER bytes of the one block in certPEM. It decodes
// the PEM itself so the check does not reuse the code under test.
func certDER(t *testing.T, certPEM []byte) []byte {
	t.Helper()
	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatalf("test certificate is not PEM")
	}
	return block.Bytes
}

// anchorFile is the anchor file name Install must write, computed from the
// certificate's SHA-256 the same way the implementation names it.
func anchorFile(der []byte) string {
	sum := sha256.Sum256(der)
	return "omt-traffic-" + strings.ToUpper(hex.EncodeToString(sum[:])) + ".crt"
}

// assertAnchor checks that the anchor file at path holds the certificate.
func assertAnchor(t *testing.T, path string, der []byte) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read anchor: %v", err)
	}
	block, _ := pem.Decode(data)
	if block == nil || !bytes.Equal(block.Bytes, der) {
		t.Fatalf("anchor %s does not hold the test certificate", path)
	}
}

// bundleHas reports whether the system bundle holds a certificate with der.
func bundleHas(t *testing.T, der []byte) bool {
	t.Helper()
	data, err := os.ReadFile(systemBundle)
	if err != nil {
		t.Fatalf("read system bundle: %v", err)
	}
	rest := data
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			return false
		}
		if block.Type == "CERTIFICATE" && bytes.Equal(block.Bytes, der) {
			return true
		}
	}
}

// assertSystemPoolTrusts checks that the pool Go's TLS uses accepts the
// throwaway root. A self-signed root verifies only when the pool holds it,
// so this shows the rebuilt bundle reached the TLS stack. Go loads the
// system pool once per process, so this must run after Install.
func assertSystemPoolTrusts(t *testing.T, der []byte) {
	t.Helper()
	pool, err := x509.SystemCertPool()
	if err != nil {
		t.Fatalf("system cert pool: %v", err)
	}
	c, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse test certificate: %v", err)
	}
	if _, err := c.Verify(x509.VerifyOptions{Roots: pool}); err != nil {
		t.Fatalf("system pool does not trust the test root: %v", err)
	}
}

// TestLiveDebianTrust installs a throwaway root in the real Debian system
// store, checks the anchor file, the rebuilt bundle and the Go TLS pool,
// then removes it and checks that all of that is gone.
func TestLiveDebianTrust(t *testing.T) {
	requireLiveRoot(t)
	certPEM := testPEM(t)
	der := certDER(t, certPEM)
	anchor := filepath.Join(debianAnchors, anchorFile(der))
	t.Cleanup(func() {
		if err := Remove(certPEM); err != nil {
			t.Errorf("cleanup Remove: %v", err)
		}
	})
	if ok, err := Trusted(certPEM); err != nil || ok {
		t.Fatalf("before Install: Trusted = %v, %v; want false, nil", ok, err)
	}
	if err := Install(certPEM); err != nil {
		t.Fatalf("Install: %v", err)
	}
	assertAnchor(t, anchor, der)
	if !bundleHas(t, der) {
		t.Fatalf("system bundle %s lacks the test certificate after Install", systemBundle)
	}
	assertSystemPoolTrusts(t, der)
	if ok, err := Trusted(certPEM); err != nil || !ok {
		t.Fatalf("after Install: Trusted = %v, %v; want true, nil", ok, err)
	}
	if err := Remove(certPEM); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if ok, err := Trusted(certPEM); err != nil || ok {
		t.Fatalf("after Remove: Trusted = %v, %v; want false, nil", ok, err)
	}
	if _, err := os.Stat(anchor); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("anchor %s after Remove: stat error %v, want not exist", anchor, err)
	}
	if bundleHas(t, der) {
		t.Fatalf("system bundle still holds the test certificate after Remove")
	}
	if err := Remove(certPEM); err != nil {
		t.Fatalf("second Remove: %v", err)
	}
}
