package procnet

import (
	"encoding/binary"
	"net/netip"
	"testing"
)

// putPort stores a port as the kernel does: in network byte order in the low
// half of a DWORD.
func putPort(b []byte, port uint16) {
	var raw [2]byte
	binary.BigEndian.PutUint16(raw[:], port)
	copy(b, raw[:])
}

func TestDecodeTCP4(t *testing.T) {
	row := make([]byte, tcp4RowSize)
	binary.NativeEndian.PutUint32(row[0:4], mibTCPEstablished)
	copy(row[4:8], []byte{127, 0, 0, 1})
	putPort(row[8:12], 5000)
	copy(row[12:16], []byte{127, 0, 0, 1})
	putPort(row[16:20], 8080)
	binary.NativeEndian.PutUint32(row[20:24], 4242)

	got := decodeTCP4(row)
	if got.local != netip.MustParseAddrPort("127.0.0.1:5000") || got.remote != netip.MustParseAddrPort("127.0.0.1:8080") {
		t.Fatalf("ends = %v -> %v", got.local, got.remote)
	}
	if got.state != mibTCPEstablished || got.pid != 4242 {
		t.Fatalf("state, pid = %d, %d", got.state, got.pid)
	}
}

func TestDecodeTCP6(t *testing.T) {
	row := make([]byte, tcp6RowSize)
	copy(row[0:16], netip.MustParseAddr("::1").AsSlice())
	putPort(row[20:24], 5000)
	copy(row[24:40], netip.MustParseAddr("::1").AsSlice())
	putPort(row[44:48], 8080)
	binary.NativeEndian.PutUint32(row[48:52], mibTCPEstablished)
	binary.NativeEndian.PutUint32(row[52:56], 99)

	got := decodeTCP6(row)
	if got.local != netip.MustParseAddrPort("[::1]:5000") || got.remote != netip.MustParseAddrPort("[::1]:8080") {
		t.Fatalf("ends = %v -> %v", got.local, got.remote)
	}
	if got.pid != 99 {
		t.Fatalf("pid = %d, want 99", got.pid)
	}
}

func TestTableRowsStopsAtBufferEnd(t *testing.T) {
	buf := make([]byte, tableHeaderSize+2*tcp4RowSize)
	binary.NativeEndian.PutUint32(buf, 5) // claims more rows than the buffer holds
	if rows := tableRows(buf, tcp4RowSize, decodeTCP4); len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
}
