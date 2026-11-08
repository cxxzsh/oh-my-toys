package pcap

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"testing"
	"time"
)

const (
	// liveEnv gates the tests that touch the real system; CI sets it.
	liveEnv = "OMT_LIVE"
	// loopbackName is the Linux loopback interface.
	loopbackName = "lo"
	// loopbackIP is the address the test listener and client use.
	loopbackIP = "127.0.0.1"
	// listenAddr asks for an ephemeral port, so the test never collides.
	listenAddr = loopbackIP + ":0"
	// liveDeadline bounds every wait, so a silent capture fails instead of
	// hanging the CI job.
	liveDeadline = 10 * time.Second
	// packetMaxAge is how far a captured packet's timestamp may be from now.
	packetMaxAge = 5 * time.Second
	// readSettle gives the reader goroutine time to enter Read, so Close
	// really interrupts a read in flight.
	readSettle = 200 * time.Millisecond
	// markerBytes is the random part of the marker; it is written as hex.
	markerBytes = 16
	// writeCount is how many small writes carry the marker.
	writeCount = 3
)

// The byte layout of an Ethernet frame carrying IPv4 and TCP, as the RFCs
// and the wire define it. The test decodes it by hand instead of trusting
// the code under test.
const (
	ethOffType    = 12
	ethHeaderLen  = 14
	etherTypeIPv4 = 0x0800

	ipv4MinHeader = 20
	ipIHLMask     = 0x0f
	ipHeaderUnit  = 4
	ipOffProto    = 9
	ipOffSrc      = 12
	ipOffDst      = 16
	ipAddrLen     = 4
	ipProtoTCP    = 6

	tcpMinHeader    = 20
	tcpOffSrcPort   = 0
	tcpOffDstPort   = 2
	tcpOffDataOff   = 12
	tcpDataOffShift = 4
	tcpWordUnit     = 4
)

// readResult is one Read call's outcome, passed from the reader goroutine.
type readResult struct {
	pkt Packet
	err error
}

// tcpFrame is the part of an Ethernet+IPv4+TCP frame the test checks.
type tcpFrame struct {
	srcIP, dstIP     net.IP
	srcPort, dstPort uint16
	payload          []byte
}

// TestLiveCaptureLoopback sends a marker over a real TCP connection on
// loopback and checks that the capture sees a matching Ethernet+IPv4+TCP
// packet. It needs CAP_NET_RAW, so CI runs the binary under sudo.
func TestLiveCaptureLoopback(t *testing.T) {
	if os.Getenv(liveEnv) == "" {
		t.Skip("set OMT_LIVE=1 to change the real system")
	}
	assertLoopbackListed(t)
	addr := listenDrain(t)
	src := openLoopback(t)
	marker := newMarker(t)
	conn := dialLoopback(t, addr.String())

	writeErr := make(chan error, 1)
	go writeMarkers(conn, marker, writeErr)

	pkt := waitForMarker(t, src, []byte(marker), writeErr)
	checkCapturedTCP(t, pkt, addr.Port, []byte(marker))
}

// TestLiveCloseUnblocksRead checks that Close returns a Read that is waiting
// on the socket, and that the Read reports io.EOF as the Source contract says.
func TestLiveCloseUnblocksRead(t *testing.T) {
	if os.Getenv(liveEnv) == "" {
		t.Skip("set OMT_LIVE=1 to change the real system")
	}
	src := openLoopback(t)

	readErr := make(chan error, 1)
	go func() {
		// Read returns a packet whenever there is traffic; keep reading
		// until the error that Close causes.
		for {
			if _, err := src.Read(); err != nil {
				readErr <- err
				return
			}
		}
	}()
	time.Sleep(readSettle)

	if err := src.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case err := <-readErr:
		if !errors.Is(err, io.EOF) {
			t.Fatalf("Read after Close = %v, want io.EOF", err)
		}
	case <-time.After(liveDeadline):
		t.Fatalf("Read still blocked %v after Close", liveDeadline)
	}
}

// assertLoopbackListed checks that Interfaces reports lo as up and loopback,
// which is what list() promises for that interface.
func assertLoopbackListed(t *testing.T) {
	t.Helper()
	ifaces, err := Interfaces()
	if err != nil {
		t.Fatalf("Interfaces: %v", err)
	}
	for _, iface := range ifaces {
		if iface.Name != loopbackName {
			continue
		}
		if !iface.Loopback || !iface.Up {
			t.Fatalf("%s: Loopback=%v Up=%v, want both true", loopbackName, iface.Loopback, iface.Up)
		}
		return
	}
	t.Fatalf("Interfaces has no %s: %+v", loopbackName, ifaces)
}

// listenDrain starts a loopback TCP listener that accepts and discards every
// connection, and returns its address.
func listenDrain(t *testing.T) *net.TCPAddr {
	t.Helper()
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", listenAddr)
	if err != nil {
		t.Fatalf("listen %s: %v", listenAddr, err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go drain(ln)
	return ln.Addr().(*net.TCPAddr)
}

// drain accepts connections until the listener closes, discarding their data.
func drain(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go func() {
			_, _ = io.Copy(io.Discard, conn)
			_ = conn.Close()
		}()
	}
}

// openLoopback opens lo for capture and closes it when the test ends.
func openLoopback(t *testing.T) Source {
	t.Helper()
	src, err := Open(loopbackName)
	if err != nil {
		t.Fatalf("Open(%s): %v (live capture needs CAP_NET_RAW: run the test binary under sudo)", loopbackName, err)
	}
	t.Cleanup(func() { _ = src.Close() })
	return src
}

// newMarker returns a random hex string no other traffic can contain.
func newMarker(t *testing.T) string {
	t.Helper()
	raw := make([]byte, markerBytes)
	if _, err := rand.Read(raw); err != nil {
		t.Fatalf("random marker: %v", err)
	}
	return hex.EncodeToString(raw)
}

// dialLoopback connects to addr and closes the connection when the test ends.
func dialLoopback(t *testing.T, addr string) net.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), liveDeadline)
	defer cancel()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// writeMarkers writes the marker in writeCount small writes. Each write holds
// the whole marker, so one packet is enough to find it. The result goes to
// errc.
func writeMarkers(conn net.Conn, marker string, errc chan<- error) {
	if err := conn.SetWriteDeadline(time.Now().Add(liveDeadline)); err != nil {
		errc <- err
		return
	}
	for i := range writeCount {
		if _, err := fmt.Fprintf(conn, "%s seq=%d\n", marker, i); err != nil {
			errc <- err
			return
		}
	}
	errc <- nil
}

// waitForMarker reads packets from src until one contains marker, failing if
// no such packet arrives within liveDeadline or if a write fails.
func waitForMarker(t *testing.T, src Source, marker []byte, writeErr <-chan error) Packet {
	t.Helper()
	pkts := startReader(t, src)
	deadline := time.NewTimer(liveDeadline)
	defer deadline.Stop()
	for {
		select {
		case r := <-pkts:
			if r.err != nil {
				t.Fatalf("read: %v", r.err)
			}
			if bytes.Contains(r.pkt.Data, marker) {
				return r.pkt
			}
		case err := <-writeErr:
			if err != nil {
				t.Fatalf("write marker: %v", err)
			}
			writeErr = nil // written: stop selecting on it
		case <-deadline.C:
			t.Fatalf("no packet with the marker within %v; the capture does not see loopback traffic", liveDeadline)
		}
	}
}

// startReader reads src in a goroutine and hands each result over a channel.
// The goroutine ends at the first read error, or when the test ends.
func startReader(t *testing.T, src Source) <-chan readResult {
	t.Helper()
	pkts := make(chan readResult)
	done := make(chan struct{})
	go func() {
		for {
			pkt, err := src.Read()
			select {
			case pkts <- readResult{pkt: pkt, err: err}:
			case <-done:
				return
			}
			if err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		close(done)
		_ = src.Close()
	})
	return pkts
}

// checkCapturedTCP checks that pkt is an Ethernet frame carrying IPv4 and TCP
// between loopback and the listener port, that its payload holds marker, and
// that its timestamp is recent.
func checkCapturedTCP(t *testing.T, pkt Packet, listenPort int, marker []byte) {
	t.Helper()
	if pkt.Link != LinkEthernet {
		t.Fatalf("link type = %v, want LinkEthernet (SOCK_RAW keeps the link header)", pkt.Link)
	}
	frame, err := parseEthernetTCP(pkt.Data)
	if err != nil {
		t.Fatalf("packet with the marker is not Ethernet+IPv4+TCP: %v", err)
	}
	loopback := net.ParseIP(loopbackIP)
	if !frame.srcIP.Equal(loopback) || !frame.dstIP.Equal(loopback) {
		t.Errorf("addresses %v -> %v, want %s both ways", frame.srcIP, frame.dstIP, loopbackIP)
	}
	if int(frame.srcPort) != listenPort && int(frame.dstPort) != listenPort {
		t.Errorf("ports %d -> %d, want one of them to be the listener %d", frame.srcPort, frame.dstPort, listenPort)
	}
	if !bytes.Contains(frame.payload, marker) {
		t.Errorf("TCP payload does not hold the marker")
	}
	if age := time.Since(pkt.Time); age.Abs() > packetMaxAge {
		t.Errorf("packet timestamp %v is %v from now, want within %v", pkt.Time, age, packetMaxAge)
	}
}

// parseEthernetTCP decodes an Ethernet frame that carries IPv4 and TCP.
func parseEthernetTCP(data []byte) (tcpFrame, error) {
	if len(data) < ethHeaderLen || binary.BigEndian.Uint16(data[ethOffType:]) != etherTypeIPv4 {
		return tcpFrame{}, errors.New("no IPv4 EtherType in the Ethernet header")
	}
	ip := data[ethHeaderLen:]
	if len(ip) < ipv4MinHeader {
		return tcpFrame{}, fmt.Errorf("IPv4 header cut to %d bytes", len(ip))
	}
	ihl := int(ip[0]&ipIHLMask) * ipHeaderUnit
	if ihl < ipv4MinHeader || len(ip) < ihl+tcpMinHeader || ip[ipOffProto] != ipProtoTCP {
		return tcpFrame{}, errors.New("IPv4 packet does not carry a whole TCP header")
	}
	tcp := ip[ihl:]
	doff := int(tcp[tcpOffDataOff]>>tcpDataOffShift) * tcpWordUnit
	if doff < tcpMinHeader || len(tcp) < doff {
		return tcpFrame{}, fmt.Errorf("TCP data offset %d is out of range", doff)
	}
	return tcpFrame{
		srcIP:   net.IP(ip[ipOffSrc : ipOffSrc+ipAddrLen]),
		dstIP:   net.IP(ip[ipOffDst : ipOffDst+ipAddrLen]),
		srcPort: binary.BigEndian.Uint16(tcp[tcpOffSrcPort:]),
		dstPort: binary.BigEndian.Uint16(tcp[tcpOffDstPort:]),
		payload: tcp[doff:],
	}, nil
}
