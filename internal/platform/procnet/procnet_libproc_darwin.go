package procnet

// The macOS binding: libproc through purego, so the build stays free of cgo,
// as internal/platform/pcap does with libpcap. The library lives in the dyld
// shared cache, which is why the path below names a file that is not on disk.

import (
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// libprocPath is the copy of libproc every Mac has.
const libprocPath = "/usr/lib/libproc.dylib"

// The flavors and sizes below come from <sys/proc_info.h> of the macOS 15 SDK.
const (
	// procPidListFDs is PROC_PIDLISTFDS: the descriptors of a process as
	// proc_fdinfo entries.
	procPidListFDs = 1
	// procPidFDSocketInfo is PROC_PIDFDSOCKETINFO: the socket behind one
	// descriptor, as socket_fdinfo.
	procPidFDSocketInfo = 3
	// procFDInfoSize is sizeof(struct proc_fdinfo): an int fd, then a uint32 type.
	procFDInfoSize = 8
	// procFDTypeSocket is PROX_FDTYPE_SOCKET.
	procFDTypeSocket = 2
	// socketFDInfoSize is sizeof(struct socket_fdinfo).
	socketFDInfoSize = 792
	// pidPathMaxSize is PROC_PIDPATHINFO_MAXSIZE, 4*MAXPATHLEN.
	pidPathMaxSize = 4096
	// sockinfoTCP is SOCKINFO_TCP, the soi_kind of a TCP socket.
	sockinfoTCP = 2
	// tcpsEstablished is TCPS_ESTABLISHED from <netinet/tcp_fsm.h>.
	tcpsEstablished = 4
	// afINET and afINET6 are the BSD address families of the two IP versions.
	afINET  = 2
	afINET6 = 30
)

// The offsets below are into socket_fdinfo, checked with offsetof against
// the SDK. proc_fileinfo is 24 bytes and comes first, then socket_info.
const (
	// fdinfoSocketInfo is where socket_info starts: after proc_fileinfo.
	fdinfoSocketInfo = 24
	// siFamily is socket_info.soi_family.
	siFamily = fdinfoSocketInfo + 160
	// siKind is socket_info.soi_kind.
	siKind = fdinfoSocketInfo + 232
	// siProto is socket_info.soi_proto, a union whose TCP view starts with
	// tcp_sockinfo.tcpsi_ini, an in_sockinfo.
	siProto = fdinfoSocketInfo + 240
	// inFport and inLport are in_sockinfo.insi_fport and insi_lport, ints
	// holding the port in network byte order.
	inFport = siProto + 0
	inLport = siProto + 4
	// inFaddr and inLaddr are the foreign and local addresses: an in6_addr,
	// or for IPv4 an in4in6_addr whose IPv4 address is its last 4 bytes.
	inFaddr = siProto + 32
	inLaddr = siProto + 48
	// tcpState is tcp_sockinfo.tcpsi_state, which follows the 80-byte
	// in_sockinfo.
	tcpState = siProto + 80
)

// libprocFuncs is the set of libproc calls the package makes; purego
// translates each Go signature to the C calling convention.
type libprocFuncs struct {
	listAllPids func(buf unsafe.Pointer, size int32) int32
	pidInfo     func(pid, flavor int32, arg uint64, buf unsafe.Pointer, size int32) int32
	pidFDInfo   func(pid, fd, flavor int32, buf unsafe.Pointer, size int32) int32
	pidPath     func(pid int32, buf unsafe.Pointer, size uint32) int32
}

// libprocAPI resolves the binding once; it is nil when libproc did not load,
// which no Mac should ever see.
var libprocAPI = sync.OnceValue(func() *libprocFuncs {
	lib, err := purego.Dlopen(libprocPath, purego.RTLD_LAZY)
	if err != nil {
		return nil
	}
	f := &libprocFuncs{}
	purego.RegisterLibFunc(&f.listAllPids, lib, "proc_listallpids")
	purego.RegisterLibFunc(&f.pidInfo, lib, "proc_pidinfo")
	purego.RegisterLibFunc(&f.pidFDInfo, lib, "proc_pidfdinfo")
	purego.RegisterLibFunc(&f.pidPath, lib, "proc_pidpath")
	return f
})
