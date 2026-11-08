// Package pcap reads the packets of a network interface. On macOS it calls
// the system's libpcap through purego, on Linux it opens an AF_PACKET
// socket, and on Windows it loads Npcap's wpcap.dll; none of them needs
// cgo. Linux is not yet verified on a real system.
package pcap

import (
	"context"
	"errors"
	"time"
)

// The errors the package returns, wrapped with detail.
var (
	// ErrPermission: the system does not let this user capture packets.
	ErrPermission = errors.New("permission denied")
	// ErrDriverMissing: the capture driver (Npcap) is not installed.
	ErrDriverMissing = errors.New("packet capture driver missing")
	// ErrUnsupported: this system has no packet capture here.
	ErrUnsupported = errors.New("packet capture is not supported on this system")
)

// LinkType says what a packet's bytes start with.
type LinkType int

// The link types.
const (
	// LinkEthernet starts with an Ethernet header.
	LinkEthernet LinkType = iota
	// LinkNull starts with the 4-byte address family of the BSD loopback.
	LinkNull
	// LinkRaw starts with the IP header.
	LinkRaw
)

// Interface is a network interface that can be read.
type Interface struct {
	Name        string
	Description string
	Up          bool
	Loopback    bool
}

// Packet is one captured packet.
type Packet struct {
	Time time.Time
	Link LinkType
	// Data is the captured bytes, at most 65535.
	Data []byte
	// Length is the packet's length on the wire.
	Length int
}

// Source is an open interface.
type Source interface {
	// Read blocks for the next packet; it returns io.EOF after Close.
	Read() (Packet, error)
	Close() error
}

// Interfaces lists the interfaces that can be read.
func Interfaces() ([]Interface, error) { return list() }

// DefaultInterface is the interface the system routes the internet through.
func DefaultInterface() (string, error) { return defaultInterface() }

// Open starts reading the interface called name.
func Open(name string) (Source, error) { return open(name) }

// Grant asks the person, through the system's own prompt, to allow packet
// capture to this user, where that is a thing (macOS: access to /dev/bpf*).
// reason is the sentence the prompt shows, already in the person's
// language, because a platform package cannot reach the i18n catalogs. It
// returns ErrUnsupported where granting is not a thing.
func Grant(ctx context.Context, reason string) error { return grant(ctx, reason) }
