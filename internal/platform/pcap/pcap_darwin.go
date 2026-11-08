package pcap

// The macOS binding: libpcap through purego, so the build stays free of
// cgo. The library lives in the dyld shared cache, which is why the path
// below names a file that is not on disk.

import (
	"fmt"
	"io"
	"slices"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
)

// libpcapPath is the copy of libpcap every Mac has.
const libpcapPath = "/usr/lib/libpcap.dylib"

// pcapTimeval is struct timeval on darwin: a long of seconds and an int of
// microseconds, padded to 16 bytes.
type pcapTimeval struct {
	sec  int64
	usec int32
	_    int32
}

// pcapHeader is struct pcap_pkthdr: when the packet was captured, how much
// of it was kept, and how long it was on the wire.
type pcapHeader struct {
	ts     pcapTimeval
	caplen uint32
	length uint32
}

// darwinFuncs is the set of libpcap calls the package makes; purego
// translates each Go signature to the C calling convention.
type darwinFuncs struct {
	findAll   func(devices **cIf, errbuf []byte) int32
	freeAll   func(devices *cIf)
	openLive  func(device string, snaplen, promisc, timeoutMs int32, errbuf []byte) unsafe.Pointer
	nextEx    func(handle unsafe.Pointer, header **pcapHeader, data **byte) int32
	datalink  func(handle unsafe.Pointer) int32
	breakLoop func(handle unsafe.Pointer)
	closeFn   func(handle unsafe.Pointer)
}

// darwinAPI resolves the binding once; it is nil when libpcap did not
// load, which no Mac should ever see.
var darwinAPI = sync.OnceValue(func() *darwinFuncs {
	lib, err := purego.Dlopen(libpcapPath, purego.RTLD_LAZY)
	if err != nil {
		return nil
	}
	f := &darwinFuncs{}
	purego.RegisterLibFunc(&f.findAll, lib, "pcap_findalldevs")
	purego.RegisterLibFunc(&f.freeAll, lib, "pcap_freealldevs")
	purego.RegisterLibFunc(&f.openLive, lib, "pcap_open_live")
	purego.RegisterLibFunc(&f.nextEx, lib, "pcap_next_ex")
	purego.RegisterLibFunc(&f.datalink, lib, "pcap_datalink")
	purego.RegisterLibFunc(&f.breakLoop, lib, "pcap_breakloop")
	purego.RegisterLibFunc(&f.closeFn, lib, "pcap_close")
	return f
})

// source is an interface open in libpcap.
type source struct {
	api    *darwinFuncs
	handle unsafe.Pointer
	name   string
	link   LinkType

	// readMu lets one Read at a time own the handle, so Close can wait for
	// the call in flight before libpcap frees it.
	readMu sync.Mutex
	closed atomic.Bool
	once   sync.Once
}

func open(name string) (Source, error) {
	api := darwinAPI()
	if api == nil {
		return nil, fmt.Errorf("pcap: open %s: %w: libpcap did not load", name, ErrUnsupported)
	}
	errbuf := make([]byte, errBufLen)
	handle := api.openLive(name, snaplen, promiscOff, readTimeoutMs, errbuf)
	if handle == nil {
		return nil, openErr(name, errText(errbuf))
	}
	link, err := pcapLink(api.datalink(handle))
	if err != nil {
		api.closeFn(handle)
		return nil, fmt.Errorf("pcap: open %s: %w", name, err)
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
		switch ret := s.api.nextEx(s.handle, &header, &data); ret {
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

// packet copies a packet out of libpcap's buffer, which the next read
// overwrites.
func (s *source) packet(header *pcapHeader, data *byte) (Packet, error) {
	if header == nil || data == nil {
		return Packet{}, fmt.Errorf("pcap: read %s: libpcap returned no packet", s.name)
	}
	captured := int(header.caplen)
	if captured > snaplen {
		return Packet{}, fmt.Errorf("pcap: read %s: captured length %d is too large", s.name, captured)
	}
	clock := time.Unix(header.ts.sec, int64(header.ts.usec)*int64(time.Microsecond))
	return Packet{
		Time: clock,
		Link: s.link,
		// The bytes are libpcap's until the next read, so they are copied.
		Data:   slices.Clone(unsafe.Slice(data, captured)), //nolint:gosec // G103: data is a pointer into libpcap's packet buffer
		Length: int(header.length),
	}, nil
}

// Close stops the read in flight and frees the handle. A read that is
// waiting is woken by pcap_breakloop, or returns on its own timeout, so
// Close never waits for a packet.
func (s *source) Close() error {
	s.once.Do(func() {
		s.closed.Store(true)
		s.api.breakLoop(s.handle)
		s.readMu.Lock()
		defer s.readMu.Unlock()
		s.api.closeFn(s.handle)
	})
	return nil
}
