//go:build darwin || linux

package core

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
)

// ListenSwitch listens on this user's socket. A leftover socket from a
// crashed app is removed first; a live app still holds the name and the
// listen fails.
func ListenSwitch(ctx context.Context) (net.Listener, error) {
	path, err := switchSock()
	if err != nil {
		return nil, err
	}
	_ = os.Remove(path)
	ln, err := (&net.ListenConfig{}).Listen(ctx, "unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = ln.Close()
		return nil, err
	}
	return ln, nil
}

// SwitchDial is how a command reaches the running desktop app.
func SwitchDial() (network, address string, err error) {
	path, err := switchSock()
	if err != nil {
		return "", "", err
	}
	return "unix", path, nil
}

func switchSock() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, "oh-my-toys")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "app.sock"), nil
}
