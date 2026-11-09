package traffic

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/platform/pcap"
	"github.com/cxxzsh/oh-my-toys/internal/platform/procnet"
	"github.com/cxxzsh/oh-my-toys/internal/platform/sysproxy"
	"github.com/cxxzsh/oh-my-toys/internal/platform/terminal"
	"github.com/cxxzsh/oh-my-toys/internal/platform/trust"
)

// The files the tool keeps under its folder.
const (
	trafficDir = "traffic"
	caFolder   = "ca"
	configFile = "config.json"
	rulesFile  = "rules.json"
	stateFile  = "sysproxy.json"
)

// brokenSuffix marks the file a damaged one was moved aside to, so that it
// neither blocks the tool nor hides what it held.
const brokenSuffix = ".broken"

// The modes the files are made with. The folder holds the root key, so
// nobody else may read it.
const (
	folderMode = 0o700
	fileMode   = 0o600
)

// system is every call the tool makes outside its own package: the system
// proxy, the trust store, the packet source and the clock. New fills it
// with the real ones; a test replaces the fields it needs with fakes, so
// that no test changes the machine.
type system struct {
	sysproxy proxySettings
	trust    trustStore
	pcap     packetSource
	now      func() time.Time
	// owner finds the program that owns a client connection of the proxy.
	owner func(local, remote netip.AddrPort) (procnet.Process, error)
	// openTerminal opens a terminal whose environment carries vars.
	openTerminal func(ctx context.Context, vars []terminal.Var) error
	// caBundles are the files that hold the system's CA bundle, in the order
	// to look for them. A machine with none gets no replacing variables.
	caBundles []string
}

// proxySettings is the system proxy of the machine.
type proxySettings struct {
	current func() (sysproxy.Snapshot, error)
	enable  func(hostport string) (sysproxy.Snapshot, error)
	restore func(sysproxy.Snapshot) error
	// restorePointing puts back only the services that still point at the
	// capture, which is what a leftover state file is undone with.
	restorePointing func(sysproxy.Snapshot, string) error
	// effective says whether the proxy apps read points at the capture: a
	// primary service with settings of its own (a proxy app's interface)
	// keeps them.
	effective func(hostport string) (bool, error)
}

// trustStore is the store the root certificate is trusted in.
type trustStore struct {
	install func(certPEM []byte) error
	remove  func(certPEM []byte) error
	trusted func(certPEM []byte) (bool, error)
}

// packetSource is where packets are read from.
type packetSource struct {
	open       func(name string) (pcap.Source, error)
	interfaces func() ([]pcap.Interface, error)
	defaultOne func() (string, error)
	grant      func(ctx context.Context, reason string) error
}

// realSystem is what the tool calls on a real machine.
func realSystem() system {
	return system{
		sysproxy: proxySettings{
			current:         sysproxy.Current,
			enable:          sysproxy.Enable,
			restore:         sysproxy.Restore,
			restorePointing: sysproxy.RestorePointing,
			effective:       sysproxy.Effective,
		},
		trust: trustStore{
			install: trust.Install,
			remove:  trust.Remove,
			trusted: trust.Trusted,
		},
		pcap: packetSource{
			open:       pcap.Open,
			interfaces: pcap.Interfaces,
			defaultOne: pcap.DefaultInterface,
			grant:      pcap.Grant,
		},
		now:          time.Now,
		owner:        procnet.Owner,
		openTerminal: terminal.Open,
		caBundles:    systemCABundles,
	}
}

// path is a file of the tool's folder.
func (t *Tool) path(name string) string { return filepath.Join(t.dir, name) }

// writeJSON writes v to path in one step: a scratch file of the same
// folder is written, flushed to the disk and renamed over path, so that a
// power cut leaves either the old file or the new one, never half of
// either.
func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", filepath.Base(path), err)
	}
	return writeBytes(path, b)
}

// writeBytes writes b to path in one step; see writeJSON.
func writeBytes(path string, b []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, folderMode); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	file, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("make a scratch file for %s: %w", path, err)
	}
	tmp := file.Name()
	defer func() { _ = os.Remove(tmp) }() // nothing is left once the rename worked
	if err := file.Chmod(fileMode); err != nil {
		_ = file.Close()
		return fmt.Errorf("set the mode of %s: %w", tmp, err)
	}
	if _, err := file.Write(b); err != nil {
		_ = file.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("flush %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	syncDir(dir)
	return nil
}

// syncDir flushes a folder's entries, so that a rename survives a power
// cut. A folder that cannot be opened or flushed costs the extra safety
// only, so the failure is not returned.
func syncDir(dir string) {
	folder, err := os.Open(dir) //nolint:gosec // a folder of the tool's own state.
	if err != nil {
		return
	}
	_ = folder.Sync()
	_ = folder.Close()
}

// readJSON reads path into v. A missing file comes back as fs.ErrNotExist,
// and a damaged one as an error that names the file.
func readJSON(path string, v any) error {
	b, err := os.ReadFile(path) //nolint:gosec // the path is a file of the tool's own folder.
	if err != nil {
		return err //nolint:wrapcheck // the caller tells a missing file from a broken one.
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}
