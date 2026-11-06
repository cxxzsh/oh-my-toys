//go:build windows

package core

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
)

// ListenSwitch listens on loopback and publishes the port next to the cache.
// Windows has no Unix socket here; the port file is local to this user.
func ListenSwitch(ctx context.Context) (net.Listener, error) {
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen ipc: %w", err)
	}
	path, err := portFile()
	if err != nil {
		_ = ln.Close()
		return nil, err
	}
	if err := os.WriteFile(path, []byte(ln.Addr().String()), 0o600); err != nil {
		_ = ln.Close()
		return nil, err
	}
	return ln, nil
}

// SwitchDial is how a command reaches the running desktop app.
func SwitchDial() (network, address string, err error) {
	path, err := portFile()
	if err != nil {
		return "", "", err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	return "tcp", string(b), nil
}

func portFile() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, "oh-my-toys")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "app.port"), nil
}
