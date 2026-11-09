package procnet

import (
	"net/netip"
	"strconv"

	"golang.org/x/sys/windows"
)

// systemPID is the pid of the NT kernel, which has no image path to read.
const systemPID = 4

// maxImagePath is the longest path Windows allows for an image, in UTF-16 units.
const maxImagePath = 32768

// owner searches the owner-PID tables of established TCP connections for the
// ends given, over IPv4 and then IPv6.
func owner(local, remote netip.AddrPort) (Process, error) {
	want := newConn(local, remote)
	for _, af := range []uint32{windows.AF_INET, windows.AF_INET6} {
		rows, err := tcpRows(af)
		if err != nil {
			return Process{}, err
		}
		for _, r := range rows {
			if r.state == mibTCPEstablished && want.matches(r.local, r.remote) {
				return windowsProcess(r.pid), nil
			}
		}
	}
	return Process{}, notFound(want)
}

// windowsProcess names the process from its image path. A process whose
// path cannot be read, which is the case for most system processes, is
// named "System" for the kernel and by its pid otherwise.
func windowsProcess(pid uint32) Process {
	path := imagePath(pid)
	if path == "" {
		name := strconv.FormatUint(uint64(pid), 10)
		if pid == systemPID {
			name = "System"
		}
		return Process{PID: int(pid), Name: name}
	}
	return Process{PID: int(pid), Name: displayName(path), Path: path}
}

// imagePath returns the full image path of the process, or "" when it cannot
// be opened with the limited query right.
func imagePath(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer func() { _ = windows.CloseHandle(h) }()
	buf := make([]uint16, maxImagePath)
	size := uint32(len(buf)) //nolint:gosec // G115: the table name buffer is 32768 units at most
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:size])
}

// tcpRows returns the rows of the owner-PID TCP table for the address family.
func tcpRows(af uint32) ([]tcpRow, error) {
	buf, err := tcpTable(af)
	if err != nil {
		return nil, err
	}
	if af == windows.AF_INET6 {
		return tableRows(buf, tcp6RowSize, decodeTCP6), nil
	}
	return tableRows(buf, tcp4RowSize, decodeTCP4), nil
}
