// Package procnet finds the program that owns a TCP connection.
package procnet

import (
	"errors"
	"net/netip"
)

// ErrNotFound means no process owns a connection with those ends: it closed
// already, or it belongs to another user and the system hides it.
var ErrNotFound = errors.New("no process owns the connection")

// ErrUnsupported means this system has no way to ask.
var ErrUnsupported = errors.New("finding the owner of a connection is not supported on this system")

// Process is the program that owns a connection.
type Process struct {
	// PID is the process id.
	PID int
	// Name is what a person calls the program: "Safari", "curl".
	Name string
	// Path is the executable's path; empty when the system does not say.
	Path string
}

// Owner returns the process that owns the TCP connection whose local end is
// local and whose peer is remote, both as that process sees them. For a
// client of the proxy, local is the client's own address and port, which the
// proxy reads as the remote address of the connection it accepted, and
// remote is the proxy's address.
func Owner(local, remote netip.AddrPort) (Process, error) { return owner(local, remote) }
