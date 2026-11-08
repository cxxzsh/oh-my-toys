//go:build windows

package sysproxy

import (
	"errors"
	"fmt"
	"math"
	"net"
	"strconv"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// internetSettingsKey is where WinINET keeps the proxy that programs read.
const internetSettingsKey = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`

// The WinINET values the package saves and writes.
const (
	valueProxyEnable   = "ProxyEnable"
	valueProxyServer   = "ProxyServer"
	valueProxyOverride = "ProxyOverride"
	valueAutoConfigURL = "AutoConfigURL"
)

// proxyOn is the ProxyEnable value that turns the proxy on.
const proxyOn = 1

// InternetSetOption options that make running programs read the settings
// again; without them a browser keeps the old proxy until it restarts.
const (
	internetOptionRefresh         = 37
	internetOptionSettingsChanged = 39
)

// wininetDLL and internetSetOption name the call that announces a change.
const (
	wininetDLL        = "wininet.dll"
	internetSetOption = "InternetSetOptionW"
)

func current() (Snapshot, error) {
	s, err := readSettings()
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{Windows: &s}, nil
}

// enable writes the manual proxy and drops the PAC URL, which would
// otherwise win over it. On a failure it puts the old settings back.
func enable(hostport string) (Snapshot, error) {
	host, port, err := parseHostPort(hostport)
	if err != nil {
		return Snapshot{}, err
	}
	old, err := readSettings()
	if err != nil {
		return Snapshot{}, err
	}
	snap := Snapshot{Windows: &old}
	server := net.JoinHostPort(host, strconv.Itoa(port))
	if err := writeEnabled(server); err != nil {
		return snap, rollback(err, writeSettings(old))
	}
	return snap, nil
}

func restore(s Snapshot) error {
	if s.Windows == nil {
		return nil
	}
	return writeSettings(*s.Windows)
}

func points(s Snapshot, hostport string) bool {
	host, port, err := parseHostPort(hostport)
	if err != nil || s.Windows == nil {
		return false
	}
	return windowsSettingsPoint(*s.Windows, host, port)
}

// effective reports true: WinINET is the one proxy setting apps of this
// user read, and it is the one Enable writes.
func effective(string) (bool, error) { return true, nil }

// restorePointing restores the saved settings only while WinINET still
// points at hostport, because a different value is the person's own change
// made after the crash.
func restorePointing(s Snapshot, hostport string) error {
	host, port, err := parseHostPort(hostport)
	if err != nil || s.Windows == nil {
		return err
	}
	current, err := readSettings()
	if err != nil {
		return err
	}
	if !windowsSettingsPoint(current, host, port) {
		return nil
	}
	return writeSettings(*s.Windows)
}

// readSettings reads the four values, remembering which ones exist.
func readSettings() (WindowsSettings, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsKey, registry.QUERY_VALUE)
	if err != nil {
		return WindowsSettings{}, settingsError("open internet settings", err)
	}
	defer closeKey(k)
	var s WindowsSettings
	if s.ProxyEnable, err = readDWORD(k, valueProxyEnable); err != nil {
		return WindowsSettings{}, err
	}
	if s.ProxyServer, err = readString(k, valueProxyServer); err != nil {
		return WindowsSettings{}, err
	}
	if s.ProxyOverride, err = readString(k, valueProxyOverride); err != nil {
		return WindowsSettings{}, err
	}
	if s.AutoConfigURL, err = readString(k, valueAutoConfigURL); err != nil {
		return WindowsSettings{}, err
	}
	return s, nil
}

func readDWORD(k registry.Key, name string) (DWORD, error) {
	value, _, err := k.GetIntegerValue(name)
	if errors.Is(err, registry.ErrNotExist) {
		return DWORD{}, nil
	}
	if err != nil {
		return DWORD{}, settingsError("read "+name, err)
	}
	if value > math.MaxUint32 {
		return DWORD{}, settingsError("read "+name, fmt.Errorf("%d does not fit a DWORD", value))
	}
	return DWORD{Present: true, Number: uint32(value)}, nil
}

func readString(k registry.Key, name string) (StringValue, error) {
	value, _, err := k.GetStringValue(name)
	if errors.Is(err, registry.ErrNotExist) {
		return StringValue{}, nil
	}
	if err != nil {
		return StringValue{}, settingsError("read "+name, err)
	}
	return StringValue{Present: true, Text: value}, nil
}

// writeEnabled points WinINET at server and announces it.
func writeEnabled(server string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsKey, registry.SET_VALUE)
	if err != nil {
		return settingsError("open internet settings", err)
	}
	defer closeKey(k)
	if err := k.SetDWordValue(valueProxyEnable, proxyOn); err != nil {
		return settingsError("write "+valueProxyEnable, err)
	}
	if err := k.SetStringValue(valueProxyServer, server); err != nil {
		return settingsError("write "+valueProxyServer, err)
	}
	if err := deleteValue(k, valueAutoConfigURL); err != nil {
		return err
	}
	return notify()
}

// writeSettings puts every saved value back, deleting the ones that did not
// exist before.
func writeSettings(s WindowsSettings) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsKey, registry.SET_VALUE)
	if err != nil {
		return settingsError("open internet settings", err)
	}
	defer closeKey(k)
	if err := writeDWORD(k, valueProxyEnable, s.ProxyEnable); err != nil {
		return err
	}
	if err := writeString(k, valueProxyServer, s.ProxyServer); err != nil {
		return err
	}
	if err := writeString(k, valueProxyOverride, s.ProxyOverride); err != nil {
		return err
	}
	if err := writeString(k, valueAutoConfigURL, s.AutoConfigURL); err != nil {
		return err
	}
	return notify()
}

func writeDWORD(k registry.Key, name string, v DWORD) error {
	if !v.Present {
		return deleteValue(k, name)
	}
	if err := k.SetDWordValue(name, v.Number); err != nil {
		return settingsError("write "+name, err)
	}
	return nil
}

func writeString(k registry.Key, name string, v StringValue) error {
	if !v.Present {
		return deleteValue(k, name)
	}
	if err := k.SetStringValue(name, v.Text); err != nil {
		return settingsError("write "+name, err)
	}
	return nil
}

func deleteValue(k registry.Key, name string) error {
	err := k.DeleteValue(name)
	if err != nil && !errors.Is(err, registry.ErrNotExist) {
		return settingsError("delete "+name, err)
	}
	return nil
}

// notify tells WinINET and the running programs that the settings changed.
// The DLL is looked up per call because the package keeps no state.
func notify() error {
	proc := windows.NewLazySystemDLL(wininetDLL).NewProc(internetSetOption)
	for _, option := range []uintptr{internetOptionSettingsChanged, internetOptionRefresh} {
		if ok, _, err := proc.Call(0, option, 0, 0); ok == 0 {
			return fmt.Errorf("internet set option %d: %w", option, err)
		}
	}
	return nil
}

// settingsError names a registry failure and reports a refused write as
// ErrPermission, which is what a locked-down profile produces.
func settingsError(op string, err error) error {
	if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		return fmt.Errorf("%s: %w", op, ErrPermission)
	}
	return fmt.Errorf("%s: %w", op, err)
}

// closeKey closes a key whose values are already written; a close failure
// cannot lose them.
func closeKey(k registry.Key) { _ = k.Close() }
