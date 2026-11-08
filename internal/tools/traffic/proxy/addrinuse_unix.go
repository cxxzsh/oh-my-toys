//go:build !windows

package proxy

import (
	"errors"
	"syscall"
)

// isAddrInUse reports whether a listen failed because another program holds
// the port.
func isAddrInUse(err error) bool { return errors.Is(err, syscall.EADDRINUSE) }
