package sysproxy

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// gnomeLiveEnv turns on the tests that change the real desktop settings.
// They never run by default, because they write to the user's proxy settings.
const gnomeLiveEnv = "OMT_LIVE"

// gnomeLiveTimeout bounds every gsettings call, so a hung D-Bus cannot hang CI.
const gnomeLiveTimeout = 30 * time.Second

// The proxy the live tests point the desktop at, and a second one the
// person "changes" to in between. Ports stay clear of common services.
const (
	gnomeLiveHost      = "127.0.0.1"
	gnomeLivePort      = "19099"
	gnomeLiveHostPort  = gnomeLiveHost + ":" + gnomeLivePort
	gnomeLiveOtherHost = "other.example.test"
	gnomeLiveOtherPort = "3128"
	gnomeLiveNoneMode  = "none"
)

// The gsettings schemas and keys the GNOME backend reads and writes. They
// are literals here, not the package constants, so the test checks the
// real names rather than trusting the code under test.
const (
	gnomeLiveProxySchema = "org.gnome.system.proxy"
	gnomeLiveHTTPSchema  = "org.gnome.system.proxy.http"
	gnomeLiveHTTPSSchema = "org.gnome.system.proxy.https"
	gnomeLiveKeyMode     = "mode"
	gnomeLiveKeyHost     = "host"
	gnomeLiveKeyPort     = "port"
)

// gnomeLiveKey names one gsettings key.
type gnomeLiveKey struct {
	schema string
	key    string
}

// gnomeLiveKeys lists every key the GNOME backend touches.
var gnomeLiveKeys = []gnomeLiveKey{
	{gnomeLiveProxySchema, gnomeLiveKeyMode},
	{gnomeLiveHTTPSchema, gnomeLiveKeyHost},
	{gnomeLiveHTTPSchema, gnomeLiveKeyPort},
	{gnomeLiveHTTPSSchema, gnomeLiveKeyHost},
	{gnomeLiveHTTPSSchema, gnomeLiveKeyPort},
}

// gnomeLiveRequire skips the test unless OMT_LIVE is set. With it set, a
// missing gsettings binary is a failure, not a skip, so CI cannot go
// silently green.
func gnomeLiveRequire(t *testing.T) {
	t.Helper()
	if os.Getenv(gnomeLiveEnv) == "" {
		t.Skip("set OMT_LIVE=1 to change the real system")
	}
	if _, err := exec.LookPath("gsettings"); err != nil {
		t.Fatalf("gsettings is required when %s is set: %v", gnomeLiveEnv, err)
	}
}

// gnomeLiveRun runs the real gsettings tool and returns its trimmed stdout.
// A failure, including a missing schema, fails the test with stderr.
func gnomeLiveRun(t *testing.T, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), gnomeLiveTimeout)
	defer cancel()
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "gsettings", args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("gsettings %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String())
}

// gnomeLiveGet reads one key with the OS tool, exactly as printed, for
// example 'manual' or 19099.
func gnomeLiveGet(t *testing.T, schema, key string) string {
	t.Helper()
	return gnomeLiveRun(t, "get", schema, key)
}

// gnomeLiveSet writes one key with the OS tool.
func gnomeLiveSet(t *testing.T, schema, key, value string) {
	t.Helper()
	gnomeLiveRun(t, "set", schema, key, value)
}

// gnomeLiveRecord reads the printed value of every key the backend touches.
func gnomeLiveRecord(t *testing.T) map[gnomeLiveKey]string {
	t.Helper()
	values := make(map[gnomeLiveKey]string, len(gnomeLiveKeys))
	for _, k := range gnomeLiveKeys {
		values[k] = gnomeLiveGet(t, k.schema, k.key)
	}
	return values
}

// gnomeLiveAssertKeys checks each key still prints the wanted value.
func gnomeLiveAssertKeys(t *testing.T, want map[gnomeLiveKey]string) {
	t.Helper()
	for k, v := range want {
		if got := gnomeLiveGet(t, k.schema, k.key); got != v {
			t.Errorf("gsettings %s %s = %s, want %s", k.schema, k.key, got, v)
		}
	}
}

// gnomeLiveEnabledKeys is what the desktop must print after Enable.
func gnomeLiveEnabledKeys() map[gnomeLiveKey]string {
	return map[gnomeLiveKey]string{
		{gnomeLiveProxySchema, gnomeLiveKeyMode}: "'manual'",
		{gnomeLiveHTTPSchema, gnomeLiveKeyHost}:  "'" + gnomeLiveHost + "'",
		{gnomeLiveHTTPSchema, gnomeLiveKeyPort}:  gnomeLivePort,
		{gnomeLiveHTTPSSchema, gnomeLiveKeyHost}: "'" + gnomeLiveHost + "'",
		{gnomeLiveHTTPSSchema, gnomeLiveKeyPort}: gnomeLivePort,
	}
}

// gnomeLiveAssertSnapshot checks a snapshot against the values the OS tool
// printed, so a backend that misreads a key is caught.
func gnomeLiveAssertSnapshot(t *testing.T, s Snapshot, printed map[gnomeLiveKey]string) {
	t.Helper()
	if s.GNOME == nil {
		t.Fatalf("snapshot has no GNOME settings")
	}
	g := s.GNOME
	if want := strings.Trim(printed[gnomeLiveKey{gnomeLiveProxySchema, gnomeLiveKeyMode}], "'"); g.Mode != want {
		t.Errorf("snapshot mode = %q, want %q", g.Mode, want)
	}
	if want := strings.Trim(printed[gnomeLiveKey{gnomeLiveHTTPSchema, gnomeLiveKeyHost}], "'"); g.HTTP.Host != want {
		t.Errorf("snapshot http host = %q, want %q", g.HTTP.Host, want)
	}
	if want := strings.Trim(printed[gnomeLiveKey{gnomeLiveHTTPSSchema, gnomeLiveKeyHost}], "'"); g.HTTPS.Host != want {
		t.Errorf("snapshot https host = %q, want %q", g.HTTPS.Host, want)
	}
	if want := printed[gnomeLiveKey{gnomeLiveHTTPSchema, gnomeLiveKeyPort}]; strconv.Itoa(g.HTTP.Port) != want {
		t.Errorf("snapshot http port = %d, want %s", g.HTTP.Port, want)
	}
	if want := printed[gnomeLiveKey{gnomeLiveHTTPSSchema, gnomeLiveKeyPort}]; strconv.Itoa(g.HTTPS.Port) != want {
		t.Errorf("snapshot https port = %d, want %s", g.HTTPS.Port, want)
	}
}

// gnomeLiveSnapshot takes the snapshot the tests restore to, and registers
// its restore so the desktop is put back even when a check fails.
func gnomeLiveSnapshot(t *testing.T) Snapshot {
	t.Helper()
	snapshot, err := Current()
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	t.Cleanup(func() {
		if err := Restore(snapshot); err != nil {
			t.Errorf("restore cleanup: %v", err)
		}
	})
	return snapshot
}

// TestLiveGNOMEEnableRestore points the real desktop proxy at a local
// address, checks the OS tool sees it, then restores and checks every key
// is back to what it printed before.
func TestLiveGNOMEEnableRestore(t *testing.T) {
	gnomeLiveRequire(t)
	before := gnomeLiveRecord(t)
	snapshot := gnomeLiveSnapshot(t)
	gnomeLiveAssertSnapshot(t, snapshot, before)

	replaced, err := Enable(gnomeLiveHostPort)
	if err != nil {
		t.Fatalf("Enable: %v", err)
	}
	gnomeLiveAssertSnapshot(t, replaced, before)
	gnomeLiveAssertKeys(t, gnomeLiveEnabledKeys())

	pointing, err := Current()
	if err != nil {
		t.Fatalf("Current after Enable: %v", err)
	}
	if !Points(pointing, gnomeLiveHostPort) {
		t.Errorf("Points(Current after Enable, %s) = false, want true", gnomeLiveHostPort)
	}

	if err := Restore(snapshot); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	gnomeLiveAssertKeys(t, before)
}

// TestLiveGNOMERestorePointing checks the crash-recovery rule on the real
// desktop: RestorePointing undoes our change while the settings still point
// at us, and leaves the settings alone once the person has changed them.
func TestLiveGNOMERestorePointing(t *testing.T) {
	gnomeLiveRequire(t)
	before := gnomeLiveRecord(t)
	snapshot := gnomeLiveSnapshot(t)

	t.Run("restores while settings still point at us", func(t *testing.T) {
		if _, err := Enable(gnomeLiveHostPort); err != nil {
			t.Fatalf("Enable: %v", err)
		}
		if err := RestorePointing(snapshot, gnomeLiveHostPort); err != nil {
			t.Fatalf("RestorePointing: %v", err)
		}
		gnomeLiveAssertKeys(t, before)
	})

	t.Run("leaves settings the person changed alone", func(t *testing.T) {
		if _, err := Enable(gnomeLiveHostPort); err != nil {
			t.Fatalf("Enable: %v", err)
		}
		changed := gnomeLivePersonChanges(t)
		if err := RestorePointing(snapshot, gnomeLiveHostPort); err != nil {
			t.Fatalf("RestorePointing: %v", err)
		}
		gnomeLiveAssertKeys(t, changed)
	})
}

// gnomeLivePersonChanges switches the proxy off and moves both schemes to
// another host, as a person might do by hand, and returns what the keys
// must print afterwards.
func gnomeLivePersonChanges(t *testing.T) map[gnomeLiveKey]string {
	t.Helper()
	gnomeLiveSet(t, gnomeLiveProxySchema, gnomeLiveKeyMode, gnomeLiveNoneMode)
	gnomeLiveSet(t, gnomeLiveHTTPSchema, gnomeLiveKeyHost, gnomeLiveOtherHost)
	gnomeLiveSet(t, gnomeLiveHTTPSchema, gnomeLiveKeyPort, gnomeLiveOtherPort)
	gnomeLiveSet(t, gnomeLiveHTTPSSchema, gnomeLiveKeyHost, gnomeLiveOtherHost)
	gnomeLiveSet(t, gnomeLiveHTTPSSchema, gnomeLiveKeyPort, gnomeLiveOtherPort)
	return map[gnomeLiveKey]string{
		{gnomeLiveProxySchema, gnomeLiveKeyMode}: "'" + gnomeLiveNoneMode + "'",
		{gnomeLiveHTTPSchema, gnomeLiveKeyHost}:  "'" + gnomeLiveOtherHost + "'",
		{gnomeLiveHTTPSchema, gnomeLiveKeyPort}:  gnomeLiveOtherPort,
		{gnomeLiveHTTPSSchema, gnomeLiveKeyHost}: "'" + gnomeLiveOtherHost + "'",
		{gnomeLiveHTTPSSchema, gnomeLiveKeyPort}: gnomeLiveOtherPort,
	}
}
