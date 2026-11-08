//go:build darwin

package pcap

// Reading the device list libpcap hands back: two linked lists of C
// structs walked by hand, with the field order a 64-bit platform gives.

import (
	"fmt"
	"unsafe"
)

// cIf is pcap_if_t: the next device, its name and description, its
// addresses, and the PCAP_IF_* flags with the padding that follows them on
// a 64-bit platform (40 bytes, checked in the tests).
type cIf struct {
	next        *cIf
	name        *byte
	description *byte
	addresses   *cAddr
	flags       uint32
	_           uint32 // the padding a 64-bit platform puts after the flags
}

// cAddr is pcap_addr: the next address, the address itself, and the three
// sockaddr pointers that follow it — the netmask, the broadcast address and
// the destination address. Only the address is read here; the other three
// are what keep the struct at 40 bytes, so the next device's entry lands
// where libpcap put it.
type cAddr struct {
	next *cAddr
	addr *cSockaddr
	_    *cSockaddr
	_    *cSockaddr
	_    *cSockaddr
}

// cSockaddr is the head of struct sockaddr on darwin; the address of a
// family follows in that family's own struct.
type cSockaddr struct {
	length uint8
	family uint8
}

// The address families and field offsets of the BSD socket structs on
// darwin: sockaddr_in holds the port then the 4-byte address, sockaddr_in6
// the port, the flow label then the 16-byte address.
const (
	afInet        = 2
	afInet6       = 30
	inAddrOffset  = 4
	in6AddrOffset = 8
	// loopbackV4 is the first octet of 127.0.0.0/8.
	loopbackV4 = 127
)

// loopbackV6 is ::1 as it sits in sockaddr_in6.
const loopbackV6 = "\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x01"

func list() ([]Interface, error) {
	api := darwinAPI()
	if api == nil {
		return nil, fmt.Errorf("pcap: list interfaces: %w: libpcap did not load", ErrUnsupported)
	}
	var head *cIf
	errbuf := make([]byte, errBufLen)
	if ret := api.findAll(&head, errbuf); ret != 0 {
		return nil, fmt.Errorf("pcap: list interfaces: %s", errText(errbuf))
	}
	if head == nil {
		return nil, nil
	}
	defer api.freeAll(head)
	return walkDevices(head), nil
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
			Loopback:    dev.flags&ifLoopback != 0 || hasLoopbackAddr(dev.addresses),
		})
	}
	return out
}

// hasLoopbackAddr reports whether one of a device's addresses is 127.0.0.0/8
// or ::1. libpcap only sets PCAP_IF_LOOPBACK from 1.9 on, so on an older
// macOS the address is the only thing that says lo0 is the loopback.
func hasLoopbackAddr(addr *cAddr) bool {
	for a := addr; a != nil; a = a.next {
		if a.addr != nil && loopbackAddr(a.addr) {
			return true
		}
	}
	return false
}

// loopbackAddr reports whether a sockaddr holds a loopback address.
func loopbackAddr(sa *cSockaddr) bool {
	switch sa.family {
	case afInet:
		return bytesAt(sa, inAddrOffset, 1)[0] == loopbackV4
	case afInet6:
		return string(bytesAt(sa, in6AddrOffset, len(loopbackV6))) == loopbackV6
	default:
		return false
	}
}

// bytesAt reads n bytes of a sockaddr, starting off bytes into it.
func bytesAt(sa *cSockaddr, off, n int) []byte {
	return unsafe.Slice((*byte)(unsafe.Add(unsafe.Pointer(sa), off)), n) //nolint:gosec // G103: the bytes of an address libpcap owns
}
