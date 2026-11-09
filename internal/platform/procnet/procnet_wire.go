package procnet

import (
	"encoding/binary"
	"fmt"
	"net/netip"
)

// conn is the pair of ends Owner looks for. Both ends are kept unmapped, so
// that ::ffff:127.0.0.1 and 127.0.0.1 are the same connection.
type conn struct{ local, remote netip.AddrPort }

// newConn returns the connection with the given ends, unmapped.
func newConn(local, remote netip.AddrPort) conn {
	return conn{unmapAddrPort(local), unmapAddrPort(remote)}
}

// matches reports whether a socket with the ends local and remote is c. A
// kernel may spell an IPv4 end as an IPv4-mapped IPv6 address, so both sides
// are unmapped before they are compared.
func (c conn) matches(local, remote netip.AddrPort) bool {
	return unmapAddrPort(local) == c.local && unmapAddrPort(remote) == c.remote
}

// unmapAddrPort turns an IPv4-mapped IPv6 address into plain IPv4.
func unmapAddrPort(ap netip.AddrPort) netip.AddrPort {
	return netip.AddrPortFrom(ap.Addr().Unmap(), ap.Port())
}

// notFound is the error for a connection no process owns.
func notFound(c conn) error {
	return fmt.Errorf("procnet: no process owns %v -> %v: %w", c.local, c.remote, ErrNotFound)
}

// addrFromWords decodes an address that the kernel stores or prints as
// 32-bit words in the host's byte order: one word for IPv4, four for IPv6. The
// result is the address whose bytes sit in memory in that order, which is the
// order the network carries them in.
func addrFromWords(words []uint32) (netip.Addr, bool) {
	var b []byte
	for _, w := range words {
		b = binary.NativeEndian.AppendUint32(b, w)
	}
	return netip.AddrFromSlice(b)
}

// netPort reads a port that the kernel keeps in the low 16 bits of a 32-bit
// field, in network byte order. Writing the field back in the host's byte
// order gives the port's two bytes as they sit in memory, and those are
// big-endian.
func netPort(field uint32) uint16 {
	var b [2]byte
	binary.NativeEndian.PutUint16(b[:], uint16(field)) //nolint:gosec // G115: the port is the low 16 bits by definition
	return binary.BigEndian.Uint16(b[:])
}
