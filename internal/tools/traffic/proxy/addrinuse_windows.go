//go:build windows

package proxy

import (
	"errors"
	"syscall"
)

// wsaAddrInUse is WSAEADDRINUSE, the value the Windows socket layer reports
// for a port another program holds. syscall.EADDRINUSE is an invented value
// on Windows and never matches.
const wsaAddrInUse syscall.Errno = 10048

// isAddrInUse reports whether a listen failed because another program holds
// the port.
func isAddrInUse(err error) bool { return errors.Is(err, wsaAddrInUse) }
