//go:build !darwin && !windows && !linux

package procnet

import "net/netip"

func owner(netip.AddrPort, netip.AddrPort) (Process, error) { return Process{}, ErrUnsupported }
