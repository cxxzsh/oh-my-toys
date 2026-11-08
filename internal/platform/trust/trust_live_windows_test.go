package trust

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

// liveEnv turns on the tests that change the real system. Each package
// keeps its own copy of this gate.
const liveEnv = "OMT_LIVE"

// liveSkipReason is the skip message without liveEnv.
const liveSkipReason = "set OMT_LIVE=1 to change the real system"

// liveCommandTimeout bounds one certutil run so a stuck dialog cannot hang
// the test.
const liveCommandTimeout = 2 * time.Minute

func liveOnly(t *testing.T) {
	t.Helper()
	if os.Getenv(liveEnv) == "" {
		t.Skip(liveSkipReason)
	}
}

// mustTrusted asks the package whether the root is trusted, failing the
// test on an error.
func mustTrusted(t *testing.T, pem []byte) bool {
	t.Helper()
	ok, err := Trusted(pem)
	if err != nil {
		t.Fatalf("Trusted: %v", err)
	}
	return ok
}

// certutilFinds asks certutil itself whether the current user's Root store
// holds the certificate with this SHA-1 hash. Its exit status is the answer:
// certutil fails when the certificate is not in the store.
func certutilFinds(t *testing.T, sha1 string) bool {
	t.Helper()
	tool, err := certutilPath()
	if err != nil {
		t.Fatalf("certutilPath: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), liveCommandTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, tool, "-user", "-store", rootStore, sha1).CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("certutil timed out: %v", ctx.Err())
	}
	if err != nil {
		t.Logf("certutil -store: %v: %s", err, outputLine(out))
		return false
	}
	return true
}

// TestLiveWindowsInstallRemove installs a throwaway root into the current
// user's Root store and removes it again, checking certutil after each step.
func TestLiveWindowsInstallRemove(t *testing.T) {
	liveOnly(t)
	pem := testPEM(t)
	c, err := parseCert(pem)
	if err != nil {
		t.Fatalf("parseCert: %v", err)
	}
	t.Cleanup(func() {
		if err := Remove(pem); err != nil {
			t.Errorf("cleanup Remove: %v", err)
		}
	})
	if mustTrusted(t, pem) {
		t.Fatal("Trusted = true before Install")
	}
	if err := Install(pem); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !mustTrusted(t, pem) {
		t.Fatal("Trusted = false after Install")
	}
	if !certutilFinds(t, c.sha1) {
		t.Fatal("certutil does not find the certificate after Install")
	}
	if err := Remove(pem); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if mustTrusted(t, pem) {
		t.Fatal("Trusted = true after Remove")
	}
	if certutilFinds(t, c.sha1) {
		t.Fatal("certutil still finds the certificate after Remove")
	}
	if err := Remove(pem); err != nil {
		t.Fatalf("second Remove: %v", err)
	}
}
