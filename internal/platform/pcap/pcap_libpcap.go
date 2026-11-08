//go:build darwin || windows

package pcap

// What every libpcap binding in this package needs to know: the numbers of
// the link types and interface flags, the sizes the library is opened
// with, and the shape of the text it reports errors in.

import (
	"bytes"
	"fmt"
	"strings"
	"unsafe"
)

// The DLT_* link types libpcap reports that the packet decoder reads.
const (
	dltNull   = 0  // DLT_NULL: a 4-byte address family header first
	dltEn10MB = 1  // DLT_EN10MB: an Ethernet header first
	dltRaw    = 12 // DLT_RAW: the IP header first
)

// The PCAP_IF_* bits of pcap_if.flags. PCAP_IF_RUNNING (4) says the link
// is up at the physical level, which Interface does not report, so it is
// not named here.
const (
	ifLoopback = 1 << 0
	ifUp       = 1 << 1
)

// What an interface is opened with: the largest frame, the interface left
// out of promiscuous mode, and a read timeout short enough that Close does
// not wait for traffic that may never come.
const (
	snaplen       = 65535
	promiscOff    = 0
	readTimeoutMs = 100
	// errBufLen is PCAP_ERRBUF_SIZE, the buffer size every libpcap call
	// with an error buffer expects.
	errBufLen = 256
)

// The values pcap_next_ex returns besides a packet.
const (
	nextOK      = 1
	nextTimeout = 0
	nextClosed  = -2 // pcap_breakloop was called, or a savefile ended
)

// pcapLink maps a DLT_* number to the link type the decoder reads.
func pcapLink(dlt int32) (LinkType, error) {
	switch dlt {
	case dltEn10MB:
		return LinkEthernet, nil
	case dltNull:
		return LinkNull, nil
	case dltRaw:
		return LinkRaw, nil
	default:
		return 0, fmt.Errorf("pcap: link type %d is not one the decoder reads", dlt)
	}
}

// errText copies the NUL-terminated message libpcap left in errbuf.
func errText(buf []byte) string {
	if i := bytes.IndexByte(buf, 0); i >= 0 {
		return string(buf[:i])
	}
	return string(buf)
}

// goString copies a NUL-terminated C string. libpcap frees the device list
// the moment it is released, so its bytes may not be kept.
func goString(p *byte) string {
	if p == nil {
		return ""
	}
	n := 0
	for *(*byte)(unsafe.Add(unsafe.Pointer(p), n)) != 0 { //nolint:gosec // G103: p is a C string libpcap owns
		n++
	}
	return string(unsafe.Slice(p, n)) //nolint:gosec // G103: the bytes of a C string libpcap owns
}

// pcapPermission reports whether libpcap's message means this user may not
// open the capture device: the BPF devices are root-only on macOS, and
// Linux wants CAP_NET_RAW.
func pcapPermission(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "permission") || strings.Contains(lower, "not permitted")
}

// openErr turns a failed pcap_open_live into the error Open returns.
func openErr(device, text string) error {
	text = strings.TrimSpace(text)
	switch {
	case pcapPermission(text):
		return fmt.Errorf("pcap: open %s: %w: %s", device, ErrPermission, text)
	case text == "":
		return fmt.Errorf("pcap: open %s: libpcap gave no reason", device)
	default:
		return fmt.Errorf("pcap: open %s: %s", device, text)
	}
}

// readErr names the failure pcap_next_ex reported.
func readErr(device string, ret int32) error {
	return fmt.Errorf("pcap: read %s: libpcap returned %d", device, ret)
}
