package procnet

import (
	"encoding/binary"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"unsafe"
)

// pidSlack is how many pids beyond the count to allocate, since processes
// may start between the count call and the list call.
const pidSlack = 64

// fdSlots is the first guess at how many descriptors a process has; the
// buffer grows when the kernel reports it is full.
const fdSlots = 256

// owner walks every process's socket descriptors and returns the first one
// whose TCP connection is established with the ends given. Processes that
// refuse to be inspected are skipped, as an unprivileged caller cannot see
// their sockets.
func owner(local, remote netip.AddrPort) (Process, error) {
	api := libprocAPI()
	if api == nil {
		return Process{}, fmt.Errorf("procnet: libproc did not load: %w", ErrUnsupported)
	}
	want := newConn(local, remote)
	for _, pid := range listPIDs(api) {
		if ownsConn(api, pid, want) {
			return darwinProcess(api, pid), nil
		}
	}
	return Process{}, notFound(want)
}

// listPIDs returns every pid the kernel reports, or nil when the list cannot
// be read.
func listPIDs(api *libprocFuncs) []int32 {
	n := api.listAllPids(nil, 0)
	if n <= 0 {
		return nil
	}
	pids := make([]int32, int(n)+pidSlack)
	got := api.listAllPids(unsafe.Pointer(&pids[0]), int32(len(pids)*4)) //nolint:gosec // G115: a pid list is far below the int32 limit
	if got <= 0 {
		return nil
	}
	return pids[:min(int(got), len(pids))]
}

// ownsConn reports whether the process has an established TCP socket with
// the ends in want.
func ownsConn(api *libprocFuncs, pid int32, want conn) bool {
	for _, fd := range socketFDs(api, pid) {
		local, remote, ok := tcpEnds(api, pid, fd)
		if ok && want.matches(local, remote) {
			return true
		}
	}
	return false
}

// socketFDs returns the descriptors of the process that are sockets.
func socketFDs(api *libprocFuncs, pid int32) []int32 {
	buf := make([]byte, fdSlots*procFDInfoSize)
	for {
		n := api.pidInfo(pid, procPidListFDs, 0, unsafe.Pointer(&buf[0]), int32(len(buf))) //nolint:gosec // G115: the buffer length is a small multiple of 8
		if n <= 0 {
			return nil
		}
		if int(n) < len(buf) {
			return socketDescriptors(buf[:n])
		}
		buf = make([]byte, 2*len(buf))
	}
}

// socketDescriptors picks the socket descriptors out of a proc_fdinfo list.
func socketDescriptors(list []byte) []int32 {
	var fds []int32
	for off := 0; off+procFDInfoSize <= len(list); off += procFDInfoSize {
		if binary.NativeEndian.Uint32(list[off+4:]) == procFDTypeSocket {
			fds = append(fds, int32(binary.NativeEndian.Uint32(list[off:]))) //nolint:gosec // G115: a descriptor number is an int32 in the kernel
		}
	}
	return fds
}

// tcpEnds returns the local and remote ends of descriptor fd when it is an
// established TCP socket.
func tcpEnds(api *libprocFuncs, pid, fd int32) (netip.AddrPort, netip.AddrPort, bool) {
	buf := make([]byte, socketFDInfoSize)
	n := api.pidFDInfo(pid, fd, procPidFDSocketInfo, unsafe.Pointer(&buf[0]), socketFDInfoSize) //nolint:gosec // G103: libproc fills the buffer through this pointer
	if n < tcpState+4 {
		return netip.AddrPort{}, netip.AddrPort{}, false
	}
	if binary.NativeEndian.Uint32(buf[siKind:]) != sockinfoTCP {
		return netip.AddrPort{}, netip.AddrPort{}, false
	}
	if binary.NativeEndian.Uint32(buf[tcpState:]) != tcpsEstablished {
		return netip.AddrPort{}, netip.AddrPort{}, false
	}
	return sockEnds(buf)
}

// sockEnds decodes the addresses and ports of an established TCP socket_fdinfo.
func sockEnds(buf []byte) (netip.AddrPort, netip.AddrPort, bool) {
	family := binary.NativeEndian.Uint32(buf[siFamily:])
	if family != afINET && family != afINET6 {
		return netip.AddrPort{}, netip.AddrPort{}, false
	}
	v4 := family == afINET
	local := netip.AddrPortFrom(sockAddr(buf, inLaddr, v4), netPort(binary.NativeEndian.Uint32(buf[inLport:])))
	remote := netip.AddrPortFrom(sockAddr(buf, inFaddr, v4), netPort(binary.NativeEndian.Uint32(buf[inFport:])))
	return local, remote, true
}

// sockAddr reads an address at off. For IPv4 the address sits in the last
// four bytes of the 16-byte in6_addr field.
func sockAddr(buf []byte, off int, v4 bool) netip.Addr {
	if v4 {
		return netip.AddrFrom4([4]byte(buf[off+12 : off+16]))
	}
	return netip.AddrFrom16([16]byte(buf[off : off+16]))
}

// darwinProcess names the process from its executable path. A process whose
// path cannot be read is named by its pid.
func darwinProcess(api *libprocFuncs, pid int32) Process {
	buf := make([]byte, pidPathMaxSize)
	n := api.pidPath(pid, unsafe.Pointer(&buf[0]), pidPathMaxSize) //nolint:gosec // G103: libproc fills the buffer through this pointer
	if n <= 0 {
		return Process{PID: int(pid), Name: strconv.Itoa(int(pid))}
	}
	path := strings.TrimRight(string(buf[:n]), "\x00")
	return Process{PID: int(pid), Name: displayName(path), Path: path}
}
