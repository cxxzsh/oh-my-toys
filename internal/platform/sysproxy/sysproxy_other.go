//go:build !darwin && !windows && !linux

package sysproxy

import "fmt"

// current returns ErrUnsupported: this system has a proxy setting this
// package does not know how to read.
func current() (Snapshot, error) {
	return Snapshot{}, fmt.Errorf("read the system proxy: %w", ErrUnsupported)
}

func enable(string) (Snapshot, error) {
	return Snapshot{}, fmt.Errorf("change the system proxy: %w", ErrUnsupported)
}

func restore(Snapshot) error {
	return fmt.Errorf("restore the system proxy: %w", ErrUnsupported)
}

func points(Snapshot, string) bool { return false }

func restorePointing(Snapshot, string) error {
	return fmt.Errorf("restore the system proxy: %w", ErrUnsupported)
}
