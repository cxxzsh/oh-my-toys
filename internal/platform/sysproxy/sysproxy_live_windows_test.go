package sysproxy

import (
	"errors"
	"os"
	"strings"
	"testing"

	"golang.org/x/sys/windows/registry"
)

// liveEnv turns on the tests that change the real system. Each package
// keeps its own copy of this gate.
const liveEnv = "OMT_LIVE"

// liveSkipReason is the skip message without liveEnv.
const liveSkipReason = "set OMT_LIVE=1 to change the real system"

// The proxy the live tests point WinINET at. The port is rarely used, so
// nothing real is cut off while it is set.
const (
	liveHostPort    = "127.0.0.1:19099"
	liveOtherServer = "10.0.0.9:8080"
	liveOtherPAC    = "http://127.0.0.1:19098/proxy.pac"
	liveOtherBypass = "localhost;<local>"
)

// The Internet Settings key and its values, spelled out here so the test
// reads the registry without the package.
const (
	liveKeyPath       = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`
	liveProxyEnable   = "ProxyEnable"
	liveProxyServer   = "ProxyServer"
	liveProxyOverride = "ProxyOverride"
	liveAutoConfig    = "AutoConfigURL"
)

// liveValue is one registry value as the test read it: whether it exists,
// and its number or text. It compares with == as a whole.
type liveValue struct {
	present bool
	num     uint64
	text    string
}

// liveRegistry is the four WinINET values as the registry holds them.
type liveRegistry struct {
	enable, server, override, autoConfig liveValue
}

func liveOnly(t *testing.T) {
	t.Helper()
	if os.Getenv(liveEnv) == "" {
		t.Skip(liveSkipReason)
	}
}

// readLiveRegistry reads the values straight from the registry, not through
// the package, so a test cannot pass by agreeing with the code under test.
func readLiveRegistry(t *testing.T) liveRegistry {
	t.Helper()
	k, err := registry.OpenKey(registry.CURRENT_USER, liveKeyPath, registry.QUERY_VALUE)
	if err != nil {
		t.Fatalf("open Internet Settings: %v", err)
	}
	defer func() { _ = k.Close() }()
	return liveRegistry{
		enable:     liveReadDWORD(t, k, liveProxyEnable),
		server:     liveReadString(t, k, liveProxyServer),
		override:   liveReadString(t, k, liveProxyOverride),
		autoConfig: liveReadString(t, k, liveAutoConfig),
	}
}

func liveReadDWORD(t *testing.T, k registry.Key, name string) liveValue {
	t.Helper()
	v, _, err := k.GetIntegerValue(name)
	if errors.Is(err, registry.ErrNotExist) {
		return liveValue{}
	}
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return liveValue{present: true, num: v}
}

func liveReadString(t *testing.T, k registry.Key, name string) liveValue {
	t.Helper()
	v, _, err := k.GetStringValue(name)
	if errors.Is(err, registry.ErrNotExist) {
		return liveValue{}
	}
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return liveValue{present: true, text: v}
}

// setLiveStrings writes string values into the Internet Settings key, the
// way another program or the person would.
func setLiveStrings(t *testing.T, values map[string]string) {
	t.Helper()
	k, err := registry.OpenKey(registry.CURRENT_USER, liveKeyPath, registry.SET_VALUE)
	if err != nil {
		t.Fatalf("open Internet Settings for writing: %v", err)
	}
	defer func() { _ = k.Close() }()
	for name, value := range values {
		if err := k.SetStringValue(name, value); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
}

// matchesRegistry reports whether a snapshot holds the registry's values.
func matchesRegistry(w WindowsSettings, r liveRegistry) bool {
	return w.ProxyEnable == DWORD{Present: r.enable.present, Number: uint32(r.enable.num)} &&
		w.ProxyServer == StringValue{Present: r.server.present, Text: r.server.text} &&
		w.ProxyOverride == StringValue{Present: r.override.present, Text: r.override.text} &&
		w.AutoConfigURL == StringValue{Present: r.autoConfig.present, Text: r.autoConfig.text}
}

// currentLive takes a snapshot and checks it against the registry first.
func currentLive(t *testing.T) Snapshot {
	t.Helper()
	snap, err := Current()
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if snap.Windows == nil {
		t.Fatal("Current: no WinINET settings")
	}
	reg := readLiveRegistry(t)
	if !matchesRegistry(*snap.Windows, reg) {
		t.Fatalf("Current = %+v, registry = %+v", *snap.Windows, reg)
	}
	return snap
}

// restoreAtCleanup puts a snapshot back when the test ends, whatever failed.
func restoreAtCleanup(t *testing.T, snap Snapshot) {
	t.Helper()
	t.Cleanup(func() {
		if err := Restore(snap); err != nil {
			t.Errorf("cleanup Restore: %v", err)
		}
	})
}

// TestLiveWindowsEnableRestore points WinINET at liveHostPort and puts the
// settings back, checking the registry directly at each step.
func TestLiveWindowsEnableRestore(t *testing.T) {
	liveOnly(t)
	snap := currentLive(t)
	restoreAtCleanup(t, snap)
	before := readLiveRegistry(t)
	t.Run("values as found", func(t *testing.T) {
		enableAndRestore(t, before)
	})
	t.Run("values set", func(t *testing.T) {
		setLiveStrings(t, map[string]string{
			liveProxyServer:   liveOtherServer,
			liveProxyOverride: liveOtherBypass,
			liveAutoConfig:    liveOtherPAC,
		})
		enableAndRestore(t, readLiveRegistry(t))
	})
}

// enableAndRestore runs Enable and Restore, checking the registry after each
// step. Enable must delete the PAC URL, and Restore must bring back what
// was there, absent values included.
func enableAndRestore(t *testing.T, want liveRegistry) {
	t.Helper()
	snap := currentLive(t)
	if _, err := Enable(liveHostPort); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	got := readLiveRegistry(t)
	if !got.enable.present || got.enable.num != proxyOn {
		t.Fatalf("ProxyEnable = %+v, want 1", got.enable)
	}
	if !strings.Contains(got.server.text, liveHostPort) {
		t.Fatalf("ProxyServer = %q, want %q", got.server.text, liveHostPort)
	}
	if got.autoConfig.present {
		t.Fatalf("AutoConfigURL = %q after Enable, want absent", got.autoConfig.text)
	}
	now := currentLive(t)
	if !Points(now, liveHostPort) {
		t.Fatalf("Points(%+v, %q) = false after Enable", now, liveHostPort)
	}
	if err := Restore(snap); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if after := readLiveRegistry(t); after != want {
		t.Fatalf("registry after Restore = %+v, want %+v", after, want)
	}
}

// TestLiveWindowsRestorePointing covers the crash-recovery path: the saved
// settings come back only while WinINET still points at liveHostPort.
func TestLiveWindowsRestorePointing(t *testing.T) {
	liveOnly(t)
	t.Run("restores while pointing at us", func(t *testing.T) {
		snap := currentLive(t)
		restoreAtCleanup(t, snap)
		want := readLiveRegistry(t)
		if _, err := Enable(liveHostPort); err != nil {
			t.Fatalf("Enable: %v", err)
		}
		if err := RestorePointing(snap, liveHostPort); err != nil {
			t.Fatalf("RestorePointing: %v", err)
		}
		if got := readLiveRegistry(t); got != want {
			t.Fatalf("registry after RestorePointing = %+v, want %+v", got, want)
		}
	})
	t.Run("leaves another server alone", func(t *testing.T) {
		snap := currentLive(t)
		restoreAtCleanup(t, snap)
		if _, err := Enable(liveHostPort); err != nil {
			t.Fatalf("Enable: %v", err)
		}
		setLiveStrings(t, map[string]string{liveProxyServer: liveOtherServer})
		want := readLiveRegistry(t)
		if err := RestorePointing(snap, liveHostPort); err != nil {
			t.Fatalf("RestorePointing: %v", err)
		}
		if got := readLiveRegistry(t); got != want {
			t.Fatalf("registry after RestorePointing = %+v, want %+v", got, want)
		}
	})
}
