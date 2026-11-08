package pcap

// The Windows binding: Npcap's wpcap.dll, which is libpcap under another
// name and a different install path, loaded through syscall so the build
// stays free of cgo. The struct layouts below are the ones a 64-bit pcap
// uses, which is what a Windows build of the app targets.
//
// Windows: not yet verified on a real system.

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

const (
	// wpcapName is the library Npcap installs; its Packet.dll sits beside
	// it in every install that has one.
	wpcapName = "wpcap.dll"
	// npcapDir is the folder Npcap uses inside the system directory.
	npcapDir = "Npcap"
	// The two LoadLibraryEx flags the search needs: the library's own
	// folder holds its dependencies, and nothing is looked for in the
	// program's folder, the current folder or on PATH.
	loadLibrarySearchDLLLoadDir = 0x00000100 // LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR
	loadLibrarySearchSystem32   = 0x00000800 // LOAD_LIBRARY_SEARCH_SYSTEM32
)

// pcapTimeval is struct timeval on Windows: two 32-bit longs, unlike the
// long and int of the BSDs.
type pcapTimeval struct {
	sec  int32
	usec int32
}

// pcapHeader is struct pcap_pkthdr, 16 bytes here because its timeval is.
type pcapHeader struct {
	ts     pcapTimeval
	caplen uint32
	length uint32
}

// cIf is pcap_if_t: the next device, its name and description, the list of
// its addresses, and the PCAP_IF_* flags with the padding that follows
// them on a 64-bit platform.
type cIf struct {
	next        *cIf
	name        *byte
	description *byte
	// The pcap_addr list of the device, which is not read: Npcap fills the
	// flag bits, so the pointer is only here to keep the offsets right.
	_     *byte
	flags uint32
	_     uint32 // the padding a 64-bit platform puts after the flags
}

// winAPI is the set of wpcap procedures the package calls.
type winAPI struct {
	findAll   *syscall.Proc
	freeAll   *syscall.Proc
	openLive  *syscall.Proc
	nextEx    *syscall.Proc
	datalink  *syscall.Proc
	breakLoop *syscall.Proc
	closeFn   *syscall.Proc
}

// winAPIResolved loads wpcap once; the error says why it is not there.
var winAPIResolved = sync.OnceValues(loadWpcap)

// loadWpcap resolves every procedure the package needs. A missing library
// or a missing dependency is ErrDriverMissing, with the loader's own error
// wrapped inside it for the log.
func loadWpcap() (*winAPI, error) {
	dll, err := loadWpcapDLL()
	if err != nil {
		return nil, fmt.Errorf("pcap: %w: %w", ErrDriverMissing, err)
	}
	f := &winAPI{}
	for _, proc := range []struct {
		name string
		into **syscall.Proc
	}{
		{"pcap_findalldevs", &f.findAll},
		{"pcap_freealldevs", &f.freeAll},
		{"pcap_open_live", &f.openLive},
		{"pcap_next_ex", &f.nextEx},
		{"pcap_datalink", &f.datalink},
		{"pcap_breakloop", &f.breakLoop},
		{"pcap_close", &f.closeFn},
	} {
		found, err := dll.FindProc(proc.name)
		if err != nil {
			return nil, fmt.Errorf("pcap: %w: %s: %w", ErrDriverMissing, proc.name, err)
		}
		*proc.into = found
	}
	return f, nil
}

// loaderFuncs is the kernel32 pair that finds and loads wpcap.
type loaderFuncs struct {
	systemDirectory *syscall.LazyProc
	loadLibrary     *syscall.LazyProc
}

// loaderAPI resolves kernel32 once. Go loads its own system DLLs from
// System32 only, so this bare name cannot be planted either.
var loaderAPI = sync.OnceValue(func() *loaderFuncs {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	return &loaderFuncs{
		systemDirectory: kernel32.NewProc("GetSystemDirectoryW"),
		loadLibrary:     kernel32.NewProc("LoadLibraryExW"),
	}
})

// loadWpcapDLL loads wpcap from an absolute path: the copy beside
// Packet.dll in the Npcap folder first, then a copy in the system folder.
// Loading it by bare name would let a wpcap.dll dropped in the program's
// folder, the current folder or anywhere on PATH run in this process.
func loadWpcapDLL() (*syscall.DLL, error) {
	loader := loaderAPI()
	system, err := loader.systemFolder()
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, path := range []string{
		filepath.Join(system, npcapDir, wpcapName),
		filepath.Join(system, wpcapName),
	} {
		dll, err := loader.loadDLL(path)
		if err == nil {
			return dll, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// systemFolder is the Windows folder that holds System32, read from the
// system rather than assumed at C:\Windows.
func (l *loaderFuncs) systemFolder() (string, error) {
	var buf [syscall.MAX_PATH]uint16
	n, _, callErr := l.systemDirectory.Call(
		uintptr(unsafe.Pointer(&buf[0])), //nolint:gosec // G103: the ABI passes the buffer's address as a word
		uintptr(len(buf)),
	)
	runtime.KeepAlive(&buf)
	if n == 0 || n >= uintptr(len(buf)) {
		return "", fmt.Errorf("pcap: GetSystemDirectory: %w", callErr)
	}
	return syscall.UTF16ToString(buf[:int(n)]), nil
}

// loadDLL loads one library by absolute path, with its dependencies taken
// from its own folder before System32 and never from the current folder.
func (l *loaderFuncs) loadDLL(path string) (*syscall.DLL, error) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, _, callErr := l.loadLibrary.Call(
		uintptr(unsafe.Pointer(name)), //nolint:gosec // G103: the ABI passes the string's address as a word
		0,
		loadLibrarySearchDLLLoadDir|loadLibrarySearchSystem32,
	)
	runtime.KeepAlive(name)
	if handle == 0 {
		return nil, fmt.Errorf("pcap: load %s: %w", path, callErr)
	}
	return &syscall.DLL{Name: path, Handle: syscall.Handle(handle)}, nil
}

// addrArg is the word a Windows procedure takes for a pointer. The ABI has
// no typed arguments, so every pointer crosses as a uintptr.
func addrArg[T any](p *T) uintptr {
	return uintptr(unsafe.Pointer(p)) //nolint:gosec // G103: the ABI passes the address as a word
}

// callVoid runs a procedure whose result nobody reads.
func callVoid(proc *syscall.Proc, args ...uintptr) {
	_, _, _ = proc.Call(args...)
}

func list() ([]Interface, error) {
	api, err := winAPIResolved()
	if err != nil {
		return nil, fmt.Errorf("pcap: list interfaces: %w", err)
	}
	var head *cIf
	errbuf := make([]byte, errBufLen)
	ret, _, _ := api.findAll.Call(addrArg(&head), addrArg(&errbuf[0]))
	runtime.KeepAlive(head)
	runtime.KeepAlive(errbuf)
	if ret != 0 {
		return nil, fmt.Errorf("pcap: list interfaces: %s", errText(errbuf))
	}
	if head == nil {
		return nil, nil
	}
	defer callVoid(api.freeAll, addrArg(head))
	return walkDevices(head), nil
}

func defaultInterface() (string, error) {
	ifaces, err := list()
	if err != nil {
		return "", err
	}
	if name, ok := pickDefault(ifaces); ok {
		return name, nil
	}
	return "", fmt.Errorf("pcap: no interface is up")
}

// walkDevices copies libpcap's device list into Go values. Nothing may
// point into the list after it is freed.
func walkDevices(head *cIf) []Interface {
	var out []Interface
	for dev := head; dev != nil; dev = dev.next {
		out = append(out, Interface{
			Name:        goString(dev.name),
			Description: goString(dev.description),
			Up:          dev.flags&ifUp != 0,
			Loopback:    dev.flags&ifLoopback != 0,
		})
	}
	return out
}

// source is an interface open in the capture driver.
type source struct {
	api    *winAPI
	handle uintptr
	name   string
	link   LinkType

	// readMu lets one Read at a time own the handle, so Close can wait for
	// the call in flight before the driver frees it.
	readMu sync.Mutex
	closed atomic.Bool
	once   sync.Once
}

func open(name string) (Source, error) {
	api, err := winAPIResolved()
	if err != nil {
		return nil, fmt.Errorf("pcap: open %s: %w", name, err)
	}
	device, err := syscall.BytePtrFromString(name)
	if err != nil {
		return nil, fmt.Errorf("pcap: open %s: %w", name, err)
	}
	errbuf := make([]byte, errBufLen)
	handle, _, _ := api.openLive.Call(
		addrArg(device),
		snaplen, promiscOff, readTimeoutMs,
		addrArg(&errbuf[0]),
	)
	runtime.KeepAlive(device)
	runtime.KeepAlive(errbuf)
	if handle == 0 {
		return nil, openErr(name, errText(errbuf))
	}
	dlt, _, _ := api.datalink.Call(handle)
	link, linkErr := pcapLink(int32(dlt)) //nolint:gosec // G115: libpcap returns an int in the low word
	if linkErr != nil {
		callVoid(api.closeFn, handle)
		return nil, fmt.Errorf("pcap: open %s: %w", name, linkErr)
	}
	return &source{api: api, handle: handle, name: name, link: link}, nil
}

// Read waits for the next packet. A read that times out starts again, so
// the timeout stays invisible to the caller while it still gives Close a
// chance to take the handle back.
func (s *source) Read() (Packet, error) {
	s.readMu.Lock()
	defer s.readMu.Unlock()
	for !s.closed.Load() {
		var header *pcapHeader
		var data *byte
		code, _, _ := s.api.nextEx.Call(s.handle, addrArg(&header), addrArg(&data))
		runtime.KeepAlive(header)
		runtime.KeepAlive(data)
		switch ret := int32(code); ret { //nolint:gosec // G115: libpcap returns an int in the low word
		case nextOK:
			return s.packet(header, data)
		case nextClosed:
			return Packet{}, io.EOF
		case nextTimeout:
			continue
		default:
			return Packet{}, readErr(s.name, ret)
		}
	}
	return Packet{}, io.EOF
}

// packet copies a packet out of the driver's buffer, which the next read
// overwrites.
func (s *source) packet(header *pcapHeader, data *byte) (Packet, error) {
	if header == nil || data == nil {
		return Packet{}, fmt.Errorf("pcap: read %s: the driver returned no packet", s.name)
	}
	captured := int(header.caplen)
	if captured > snaplen {
		return Packet{}, fmt.Errorf("pcap: read %s: captured length %d is too large", s.name, captured)
	}
	clock := time.Unix(int64(header.ts.sec), int64(header.ts.usec)*int64(time.Microsecond))
	return Packet{
		Time: clock,
		Link: s.link,
		// The bytes are the driver's until the next read, so they are
		// copied.
		Data:   slices.Clone(unsafe.Slice(data, captured)), //nolint:gosec // G103: data is a pointer into the driver's packet buffer
		Length: int(header.length),
	}, nil
}

// Close stops the read in flight and frees the handle. A read that is
// waiting is woken by pcap_breakloop, or returns on its own timeout, so
// Close never waits for a packet.
func (s *source) Close() error {
	s.once.Do(func() {
		s.closed.Store(true)
		callVoid(s.api.breakLoop, s.handle)
		s.readMu.Lock()
		defer s.readMu.Unlock()
		callVoid(s.api.closeFn, s.handle)
	})
	return nil
}

func grant(context.Context, string) error {
	return fmt.Errorf("pcap: grant: %w", ErrUnsupported)
}
