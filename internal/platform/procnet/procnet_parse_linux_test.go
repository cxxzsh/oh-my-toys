package procnet

import (
	"fmt"
	"net/netip"
	"testing"
)

// kernelWord prints four address bytes the way /proc/net/tcp does: as the
// host's 32-bit integer in upper-case hex.
func kernelWord(b []byte) string {
	return fmt.Sprintf("%08X", memWord(b))
}

func TestParseHexAddr(t *testing.T) {
	v4 := netip.MustParseAddr("127.0.0.1").As4()
	v6 := netip.MustParseAddr("2001:db8::1").As16()
	mapped := netip.MustParseAddr("::ffff:192.0.2.7").As16()

	tests := []struct {
		name, hex, want string
	}{
		{"ipv4", kernelWord(v4[:]), "127.0.0.1"},
		{"ipv6", kernelWord(v6[0:4]) + kernelWord(v6[4:8]) + kernelWord(v6[8:12]) + kernelWord(v6[12:16]), "2001:db8::1"},
		{"ipv4 mapped in ipv6", kernelWord(mapped[0:4]) + kernelWord(mapped[4:8]) + kernelWord(mapped[8:12]) + kernelWord(mapped[12:16]), "::ffff:192.0.2.7"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseHexAddr(tt.hex)
			if err != nil {
				t.Fatalf("parseHexAddr(%q): %v", tt.hex, err)
			}
			if got != netip.MustParseAddr(tt.want) {
				t.Fatalf("parseHexAddr(%q) = %v, want %v", tt.hex, got, tt.want)
			}
		})
	}
	if _, err := parseHexAddr("0100"); err == nil {
		t.Fatal("a short address must fail")
	}
}

func TestParseProcNetTable(t *testing.T) {
	v4 := netip.MustParseAddr("127.0.0.1").As4()
	addr := kernelWord(v4[:])
	table := "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n" +
		"   0: " + addr + ":1F90 " + addr + ":D2F4 01 00000000:00000000 00:00000000 00000000  1000        0 12345 1 0 0 0\n" +
		"   1: " + addr + ":1F91 " + addr + ":D2F5 06 00000000:00000000 00:00000000 00000000  1000        0 67890 1 0 0 0\n"

	socks := parseProcNetTable(table)
	if len(socks) != 2 {
		t.Fatalf("parsed %d rows, want 2 (header skipped)", len(socks))
	}
	est := socks[0]
	if !est.established || est.inode != 12345 {
		t.Fatalf("row 0 = %+v, want established inode 12345", est)
	}
	if est.local != netip.MustParseAddrPort("127.0.0.1:8080") || est.remote != netip.MustParseAddrPort("127.0.0.1:54004") {
		t.Fatalf("row 0 ends = %v -> %v", est.local, est.remote)
	}
	if socks[1].established {
		t.Fatal("row 1 (state 06, TIME_WAIT) must not be established")
	}
}
