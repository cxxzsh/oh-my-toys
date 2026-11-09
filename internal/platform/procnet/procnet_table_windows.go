package procnet

import (
	"encoding/binary"
	"fmt"
	"net/netip"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	iphlpapi            = windows.NewLazySystemDLL("iphlpapi.dll")
	getExtendedTCPTable = iphlpapi.NewProc("GetExtendedTcpTable")
)

const (
	// tcpTableOwnerPIDAll is TCP_TABLE_OWNER_PID_ALL: every connection, with
	// the pid of its owner.
	tcpTableOwnerPIDAll = 5
	// mibTCPEstablished is MIB_TCP_STATE_ESTAB, the state of an established
	// connection in the MIB tables.
	mibTCPEstablished = 5
	// errorInsufficientBuffer is ERROR_INSUFFICIENT_BUFFER: the table grew
	// between the size call and the fill call.
	errorInsufficientBuffer = 122
	// tableAttempts bounds how many times the table is re-read when it grows.
	tableAttempts = 3
	// tableHeaderSize is the dwNumEntries field that starts every table.
	tableHeaderSize = 4
	// tcp4RowSize and tcp6RowSize are sizeof(MIB_TCPROW_OWNER_PID) and
	// sizeof(MIB_TCP6ROW_OWNER_PID).
	tcp4RowSize = 24
	tcp6RowSize = 56
)

// tcpRow is one connection of an owner-PID table.
type tcpRow struct {
	local, remote netip.AddrPort
	state         uint32
	pid           uint32
}

// tcpTable returns the owner-PID table for the address family, sized by a
// first call and filled by a second, retried if the table grows meanwhile.
func tcpTable(af uint32) ([]byte, error) {
	var buf []byte
	var size uint32
	for range tableAttempts {
		var ptr uintptr
		if len(buf) > 0 {
			ptr = uintptr(unsafe.Pointer(&buf[0])) //nolint:gosec // G103: the syscall needs the buffer address
		}
		code, _, _ := getExtendedTCPTable.Call(ptr, uintptr(unsafe.Pointer(&size)), 0, uintptr(af), tcpTableOwnerPIDAll, 0) //nolint:gosec // G103: the syscall writes the table size through this pointer
		switch code {
		case 0:
			return buf[:size], nil
		case errorInsufficientBuffer:
			buf = make([]byte, size)
		default:
			return nil, fmt.Errorf("procnet: GetExtendedTcpTable: %w", windows.Errno(code))
		}
	}
	return nil, fmt.Errorf("procnet: TCP table kept growing")
}

// tableRows decodes the rows that follow the table header. A row that would
// run past the end of the buffer ends the table.
func tableRows(buf []byte, rowSize int, decode func([]byte) tcpRow) []tcpRow {
	if len(buf) < tableHeaderSize {
		return nil
	}
	count := int(binary.NativeEndian.Uint32(buf))
	rows := make([]tcpRow, 0, count)
	for i := range count {
		off := tableHeaderSize + i*rowSize
		if off+rowSize > len(buf) {
			break
		}
		rows = append(rows, decode(buf[off:off+rowSize]))
	}
	return rows
}

// decodeTCP4 reads a MIB_TCPROW_OWNER_PID. Addresses are stored as the bytes
// of an in_addr, so they are read as-is. Ports are in network byte order in
// the low half of their DWORD.
func decodeTCP4(r []byte) tcpRow {
	return tcpRow{
		state: binary.NativeEndian.Uint32(r[0:4]),
		local: netip.AddrPortFrom(netip.AddrFrom4([4]byte(r[4:8])), netPort(binary.NativeEndian.Uint32(r[8:12]))),
		remote: netip.AddrPortFrom(netip.AddrFrom4([4]byte(r[12:16])),
			netPort(binary.NativeEndian.Uint32(r[16:20]))),
		pid: binary.NativeEndian.Uint32(r[20:24]),
	}
}

// decodeTCP6 reads a MIB_TCP6ROW_OWNER_PID. The scope IDs are not needed to
// tell connections apart, so they are skipped.
func decodeTCP6(r []byte) tcpRow {
	return tcpRow{
		local: netip.AddrPortFrom(netip.AddrFrom16([16]byte(r[0:16])), netPort(binary.NativeEndian.Uint32(r[20:24]))),
		remote: netip.AddrPortFrom(netip.AddrFrom16([16]byte(r[24:40])),
			netPort(binary.NativeEndian.Uint32(r[44:48]))),
		state: binary.NativeEndian.Uint32(r[48:52]),
		pid:   binary.NativeEndian.Uint32(r[52:56]),
	}
}
