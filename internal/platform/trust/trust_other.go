//go:build !darwin && !windows && !linux

package trust

import (
	"fmt"
	"os/exec"
)

// configure leaves the command as it is: these systems have no console
// window to keep out of sight.
func configure(cmd *exec.Cmd) *exec.Cmd { return cmd }

// install returns ErrUnsupported: this system has no trust store the
// package can change.
func install(cert) error { return fmt.Errorf("install certificate: %w", ErrUnsupported) }

// remove returns ErrUnsupported: this system has no trust store the package
// can change.
func remove(cert) error { return fmt.Errorf("remove certificate: %w", ErrUnsupported) }

// trusted returns ErrUnsupported: this system has no trust store the
// package can change.
func trusted(cert) (bool, error) {
	return false, fmt.Errorf("check trusted certificate: %w", ErrUnsupported)
}
