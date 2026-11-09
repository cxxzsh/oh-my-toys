package procnet

import (
	"encoding/binary"
	"net/netip"
	"testing"
)

// memWord returns the 32-bit word the kernel holds for the four bytes at b,
// which is how an address word looks before it is printed or copied.
func memWord(b []byte) uint32 {
	return binary.NativeEndian.Uint32(b)
}

func TestAddrFromWords(t *testing.T) {
	v4 := netip.MustParseAddr("127.0.0.1").As4()
	if got, ok := addrFromWords([]uint32{memWord(v4[:])}); !ok || got != netip.MustParseAddr("127.0.0.1") {
		t.Fatalf("IPv4 words = %v, %v; want 127.0.0.1", got, ok)
	}

	v6 := netip.MustParseAddr("2001:db8::1").As16()
	words := make([]uint32, 0, 4)
	for i := 0; i < 16; i += 4 {
		words = append(words, memWord(v6[i:i+4]))
	}
	if got, ok := addrFromWords(words); !ok || got != netip.MustParseAddr("2001:db8::1") {
		t.Fatalf("IPv6 words = %v, %v; want 2001:db8::1", got, ok)
	}

	if _, ok := addrFromWords([]uint32{1, 2}); ok {
		t.Fatal("two words should not form an address")
	}
}

func TestNetPort(t *testing.T) {
	// The kernel keeps the port in network byte order, so the first byte of
	// the field is the high byte of the port.
	var field [4]byte
	binary.BigEndian.PutUint16(field[:2], 8080)
	if got := netPort(memWord(field[:])); got != 8080 {
		t.Fatalf("netPort = %d, want 8080", got)
	}
	binary.BigEndian.PutUint16(field[:2], 443)
	if got := netPort(memWord(field[:])); got != 443 {
		t.Fatalf("netPort = %d, want 443", got)
	}
}

func TestConnMatchesMappedAddresses(t *testing.T) {
	local := netip.MustParseAddrPort("127.0.0.1:5000")
	remote := netip.MustParseAddrPort("127.0.0.1:8080")
	c := newConn(local, remote)

	mappedLocal := netip.AddrPortFrom(netip.MustParseAddr("::ffff:127.0.0.1"), 5000)
	if !c.matches(mappedLocal, remote) {
		t.Fatal("IPv4-mapped local end should match its IPv4 form")
	}
	if c.matches(netip.MustParseAddrPort("127.0.0.1:5001"), remote) {
		t.Fatal("a different local port must not match")
	}
	if c.matches(local, netip.MustParseAddrPort("127.0.0.2:8080")) {
		t.Fatal("a different remote address must not match")
	}
}
