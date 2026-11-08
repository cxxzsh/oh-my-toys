package pcap

// The Linux binding: an AF_PACKET raw socket per interface, which needs
// the kernel's permission to see other programs' traffic.
//
// Linux: not yet verified on a real system.

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"
)

const (
	// ethPAll is ETH_P_ALL: every protocol on the link.
	ethPAll = 0x0003
	// frameMax is the largest frame a read buffer holds.
	frameMax = 65535
	// recvTimeoutMs is how often a read that finds nothing returns, so
	// Close can take the socket back.
	recvTimeoutMs = 100
	// procRoute is where the kernel keeps the IPv4 routing table.
	procRoute = "/proc/net/route"
	// rtfUp is RTF_UP in the Flags column of a route.
	rtfUp = 0x0001
	// noRoute is the destination and the mask of the default route, both
	// written as hexadecimal in /proc/net/route.
	noRoute = "00000000"
)

// The columns of a /proc/net/route row the default route is read from.
const (
	colIface  = 0
	colDest   = 1
	colFlags  = 3
	colMetric = 6
	colMask   = 7
	// routeFields is how many columns are needed before the mask is there.
	routeFields = colMask + 1
)

func list() ([]Interface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("pcap: list interfaces: %w", err)
	}
	out := make([]Interface, 0, len(ifaces))
	for _, iface := range ifaces {
		out = append(out, Interface{
			Name:     iface.Name,
			Up:       iface.Flags&net.FlagUp != 0,
			Loopback: iface.Flags&net.FlagLoopback != 0,
		})
	}
	return out, nil
}

func defaultInterface() (string, error) {
	data, readErr := os.ReadFile(procRoute)
	if readErr == nil {
		if name := procRouteInterface(data); name != "" {
			return name, nil
		}
		readErr = errors.New("the routing table has no default route")
	}
	ifaces, err := list()
	if err != nil {
		return "", err
	}
	if name, ok := pickDefault(ifaces); ok {
		return name, nil
	}
	return "", fmt.Errorf("pcap: default interface: %w", readErr)
}

// procRouteInterface reads the interface of the IPv4 default route: of the
// rows that go everywhere, are up, and have the lowest metric.
//
//	Iface  Destination Gateway    Flags RefCnt Use Metric Mask       MTU ...
//	enp0s3 00000000    0102A8C0   0003  0      0   100    00000000   0   ...
func procRouteInterface(data []byte) string {
	best, bestMetric, found := "", uint64(0), false
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Scan() // the column header
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < routeFields || !isDefaultRoute(fields) {
			continue
		}
		metric, err := strconv.ParseUint(fields[colMetric], 10, 32)
		if err != nil {
			continue
		}
		if !found || metric < bestMetric {
			best, bestMetric, found = fields[colIface], metric, true
		}
	}
	return best
}

// isDefaultRoute reports whether a row of the routing table is an up route
// to every address.
func isDefaultRoute(fields []string) bool {
	if fields[colDest] != noRoute || fields[colMask] != noRoute {
		return false
	}
	flags, err := strconv.ParseUint(fields[colFlags], 16, 32)
	return err == nil && flags&rtfUp != 0
}

// source is an interface read through a raw socket.
type source struct {
	fd   int
	name string
	link LinkType
	buf  []byte

	// readMu lets one Read at a time own the socket, so Close can wait for
	// the call in flight before the kernel frees it.
	readMu   sync.Mutex
	closed   atomic.Bool
	once     sync.Once
	closeErr error
}

func open(name string) (Source, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return nil, fmt.Errorf("pcap: open %s: %w", name, err)
	}
	fd, err := unix.Socket(unix.AF_PACKET, unix.SOCK_RAW|unix.SOCK_CLOEXEC, int(htons(ethPAll)))
	if err != nil {
		return nil, socketErr(name, err)
	}
	s := &source{fd: fd, name: name, link: LinkEthernet, buf: make([]byte, frameMax)}
	if err := s.setup(iface.Index); err != nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("pcap: open %s: %w", name, err)
	}
	return s, nil
}

// setup binds the socket to the interface and gives it a read timeout, so
// a read that finds nothing returns instead of blocking Close forever.
func (s *source) setup(index int) error {
	if err := unix.Bind(s.fd, &unix.SockaddrLinklayer{Protocol: htons(ethPAll), Ifindex: index}); err != nil {
		return err
	}
	timeout := unix.NsecToTimeval(recvTimeoutMs * time.Millisecond.Nanoseconds())
	return unix.SetsockoptTimeval(s.fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &timeout)
}

// htons puts a 16-bit protocol number in network byte order, the way the
// socket call and the link-layer address both expect it.
func htons(protocol uint16) uint16 {
	return protocol<<8 | protocol>>8
}

// socketErr names a failed socket(2): EPERM and EACCES mean this user may
// not capture, which the UI turns into the permission guidance.
func socketErr(name string, err error) error {
	if errors.Is(err, unix.EPERM) || errors.Is(err, unix.EACCES) {
		return fmt.Errorf("pcap: open %s: %w: %w", name, ErrPermission, err)
	}
	return fmt.Errorf("pcap: open %s: %w", name, err)
}

// Read waits for the next frame. SOCK_RAW keeps the link header, so every
// frame is an Ethernet frame, loopback included.
func (s *source) Read() (Packet, error) {
	s.readMu.Lock()
	defer s.readMu.Unlock()
	for !s.closed.Load() {
		n, _, err := unix.Recvfrom(s.fd, s.buf, 0)
		switch {
		case err == nil:
			return Packet{
				Time:   time.Now(),
				Link:   s.link,
				Data:   slices.Clone(s.buf[:n]),
				Length: n,
			}, nil
		case errors.Is(err, unix.EAGAIN), errors.Is(err, unix.EINTR):
			continue
		default:
			return Packet{}, fmt.Errorf("pcap: read %s: %w", s.name, err)
		}
	}
	return Packet{}, io.EOF
}

// Close stops the read in flight and frees the socket. A read that is
// waiting returns on its own timeout, so Close never waits for a packet.
func (s *source) Close() error {
	s.once.Do(func() {
		s.closed.Store(true)
		s.readMu.Lock()
		defer s.readMu.Unlock()
		s.closeErr = unix.Close(s.fd)
	})
	return s.closeErr
}

func grant(context.Context, string) error {
	return fmt.Errorf("pcap: grant: %w: packets are captured through the kernel's own permission", ErrUnsupported)
}
