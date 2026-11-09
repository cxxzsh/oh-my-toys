package procnet

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"
)

// tcpEstablished is the state code of an established socket, the kernel's
// TCP_ESTABLISHED, as /proc/net/tcp prints it in hex.
const tcpEstablished = 1

// hexWordLen is the width of one 32-bit word in an address, as %08X prints it.
const hexWordLen = 8

// procSocket is one row of /proc/net/tcp or /proc/net/tcp6.
type procSocket struct {
	local, remote netip.AddrPort
	established   bool
	inode         uint64
}

// parseProcNetTable reads every row of one table. The header row and any row
// that does not parse are skipped.
func parseProcNetTable(data string) []procSocket {
	var socks []procSocket
	for _, line := range strings.Split(data, "\n") {
		s, err := parseProcNetLine(line)
		if err != nil {
			continue
		}
		socks = append(socks, s)
	}
	return socks
}

// parseProcNetLine reads one row. The columns are: slot, local address,
// remote address, state, queues, timers, retransmits, uid, timeout, inode.
func parseProcNetLine(line string) (procSocket, error) {
	f := strings.Fields(line)
	if len(f) < 10 {
		return procSocket{}, fmt.Errorf("procnet: short /proc/net/tcp row %q", line)
	}
	local, err := parseEndpoint(f[1])
	if err != nil {
		return procSocket{}, err
	}
	remote, err := parseEndpoint(f[2])
	if err != nil {
		return procSocket{}, err
	}
	state, err := strconv.ParseUint(f[3], 16, 8)
	if err != nil {
		return procSocket{}, fmt.Errorf("procnet: state %q: %w", f[3], err)
	}
	inode, err := strconv.ParseUint(f[9], 10, 64)
	if err != nil {
		return procSocket{}, fmt.Errorf("procnet: inode %q: %w", f[9], err)
	}
	return procSocket{local: local, remote: remote, established: state == tcpEstablished, inode: inode}, nil
}

// parseEndpoint reads "ADDRESS:PORT" where both are hex and the port is
// plain, not network order.
func parseEndpoint(s string) (netip.AddrPort, error) {
	host, portHex, ok := strings.Cut(s, ":")
	if !ok {
		return netip.AddrPort{}, fmt.Errorf("procnet: endpoint %q has no port", s)
	}
	port, err := strconv.ParseUint(portHex, 16, 16)
	if err != nil {
		return netip.AddrPort{}, fmt.Errorf("procnet: port in %q: %w", s, err)
	}
	addr, err := parseHexAddr(host)
	if err != nil {
		return netip.AddrPort{}, err
	}
	return netip.AddrPortFrom(addr, uint16(port)), nil
}

// parseHexAddr reads an address the kernel printed as 32-bit words: one word
// for IPv4, four for IPv6. Each word is printed as the integer the kernel
// holds, so it is decoded with the host's byte order.
func parseHexAddr(h string) (netip.Addr, error) {
	if len(h) != hexWordLen && len(h) != 4*hexWordLen {
		return netip.Addr{}, fmt.Errorf("procnet: address %q has %d hex digits", h, len(h))
	}
	words := make([]uint32, 0, len(h)/hexWordLen)
	for i := 0; i < len(h); i += hexWordLen {
		w, err := strconv.ParseUint(h[i:i+hexWordLen], 16, 32)
		if err != nil {
			return netip.Addr{}, fmt.Errorf("procnet: address word in %q: %w", h, err)
		}
		words = append(words, uint32(w))
	}
	addr, ok := addrFromWords(words)
	if !ok {
		return netip.Addr{}, fmt.Errorf("procnet: address %q has no valid length", h)
	}
	return addr, nil
}
