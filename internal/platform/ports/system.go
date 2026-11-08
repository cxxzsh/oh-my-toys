//go:build darwin || linux

package ports

import "strings"

// isSystemPath reports whether an executable runs from an OS directory. Only
// the systems whose listing reports the path use it: Windows does not set
// Listener.Path.
func isSystemPath(path string) bool {
	for _, p := range []string{"/System/", "/usr/libexec/", "/usr/sbin/", "/sbin/"} {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}
