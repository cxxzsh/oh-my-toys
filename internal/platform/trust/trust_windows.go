package trust

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/windows"
)

// The store that holds the current user's roots, and the tool that changes
// it.
const (
	certutilName = "certutil.exe"
	rootStore    = "Root"
)

// certutilPath is the absolute path of certutil, taken from the system
// directory and never from PATH: a tool planted earlier on PATH could fake
// the store, or the confirmation dialog, and keep the certificate.
func certutilPath() (string, error) {
	dir, err := windows.GetSystemDirectory()
	if err != nil {
		return "", fmt.Errorf("find the system directory: %w", err)
	}
	return filepath.Join(dir, certutilName), nil
}

// configure keeps certutil from flashing a console window over the app: it
// is a console program, and the app is not.
func configure(cmd *exec.Cmd) *exec.Cmd {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd
}

// install adds the certificate to the current user's Root store. Windows
// shows its own security warning for a root certificate.
func install(c cert) error {
	tool, err := certutilPath()
	if err != nil {
		return err
	}
	return installWith(c, newRunner(), tool)
}

// installWith is install with the command runner and the tool to test it
// through.
func installWith(c cert, r *runner, tool string) error {
	return withTempPEM(c, func(path string) error {
		out, err := r.exec(tool, "-user", "-f", "-addstore", rootStore, path)
		switch {
		case err == nil:
			return nil
		case permissionDenied(out):
			return fmt.Errorf("add trusted certificate: %w", ErrPermission)
		default:
			return fmt.Errorf("add trusted certificate: %w: %s", err, outputLine(out))
		}
	})
}

// remove takes the certificate out of the current user's Root store.
func remove(c cert) error {
	tool, err := certutilPath()
	if err != nil {
		return err
	}
	return removeWith(c, newRunner(), tool)
}

// removeWith is remove with the command runner and the tool to test it
// through.
func removeWith(c cert, r *runner, tool string) error {
	out, err := r.exec(tool, "-user", "-delstore", rootStore, c.sha1)
	switch {
	case err == nil, itemNotFound(out):
		return nil
	case permissionDenied(out):
		return fmt.Errorf("delete trusted certificate: %w", ErrPermission)
	default:
		return fmt.Errorf("delete trusted certificate: %w: %s", err, outputLine(out))
	}
}

// trusted reports whether the certificate is in the current user's Root
// store, which is where Windows trusts a root for TLS servers from.
func trusted(c cert) (bool, error) {
	tool, err := certutilPath()
	if err != nil {
		return false, err
	}
	return trustedWith(c, newRunner(), tool)
}

// trustedWith is trusted with the command runner and the tool to test it
// through.
func trustedWith(c cert, r *runner, tool string) (bool, error) {
	out, err := r.exec(tool, "-user", "-verifystore", rootStore, c.sha1)
	switch {
	case err == nil:
		return true, nil
	case itemNotFound(out):
		return false, nil
	case permissionDenied(out):
		return false, fmt.Errorf("check trusted certificate: %w", ErrPermission)
	default:
		return false, fmt.Errorf("check trusted certificate: %w: %s", err, outputLine(out))
	}
}

// permissionDenied reports whether certutil said the person refused its
// confirmation or lacked the right to the store. certutil has no status
// code for it, only words.
func permissionDenied(out []byte) bool {
	return says(out, "denied", "cancel", "0x80070005", "0x800704c7")
}

// itemNotFound reports whether certutil said the certificate is not in the
// store; removing an absent certificate is not a failure.
func itemNotFound(out []byte) bool {
	return says(out, "cannot find", "not found", "does not exist", "no more items", "error_not_found", "0x80092004")
}
