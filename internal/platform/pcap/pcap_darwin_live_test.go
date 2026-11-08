//go:build darwin

package pcap

// What the real machine reports: the device list, the default route, and a
// live capture on that interface, which is skipped without the permission
// to read /dev/bpf*.

import (
	"errors"
	"io"
	"slices"
	"testing"
	"time"
)

// TestInterfaces reads the real device list: a machine always has at least
// one interface, and the loopback is never reported as an ordinary one.
func TestInterfaces(t *testing.T) {
	ifaces, err := Interfaces()
	if err != nil {
		t.Fatalf("list interfaces: %v", err)
	}
	if len(ifaces) == 0 {
		t.Fatal("libpcap listed no interface")
	}
	for _, iface := range ifaces {
		if iface.Name == "" {
			t.Errorf("an interface has no name: %+v", iface)
		}
		if iface.Name == "lo0" && !iface.Loopback {
			t.Errorf("lo0 is reported as an ordinary interface: %+v", iface)
		}
	}
	if !slices.ContainsFunc(ifaces, func(iface Interface) bool { return iface.Loopback }) {
		t.Error("no interface is reported as the loopback")
	}
}

// TestDefaultInterface reads the real default route and checks it names an
// interface libpcap also lists.
func TestDefaultInterface(t *testing.T) {
	name, err := DefaultInterface()
	if err != nil {
		t.Fatalf("default interface: %v", err)
	}
	ifaces, err := Interfaces()
	if err != nil {
		t.Fatalf("list interfaces: %v", err)
	}
	if !slices.ContainsFunc(ifaces, func(iface Interface) bool { return iface.Name == name }) {
		t.Errorf("default interface %q is not one of %v", name, names(ifaces))
	}
}

func names(ifaces []Interface) []string {
	out := make([]string, 0, len(ifaces))
	for _, iface := range ifaces {
		out = append(out, iface.Name)
	}
	return out
}

// TestOpenReadsAndCloses opens the real default interface. Without the
// permission to read /dev/bpf* it skips; with it, it reads for a moment
// and then proves that Close frees a read that is waiting.
func TestOpenReadsAndCloses(t *testing.T) {
	name, err := DefaultInterface()
	if err != nil {
		t.Skipf("no default interface to read: %v", err)
	}
	src, err := Open(name)
	if errors.Is(err, ErrPermission) {
		t.Skipf("this user may not capture packets: %v", err)
	}
	if err != nil {
		t.Fatalf("open %s: %v", name, err)
	}
	defer func() {
		if cerr := src.Close(); cerr != nil {
			t.Errorf("close %s: %v", name, cerr)
		}
	}()

	packets, stopped := readInBackground(src)
	seen := collect(t, packets, stopped)
	if err := src.Close(); err != nil {
		t.Fatalf("close %s: %v", name, err)
	}
	expectEOF(t, stopped)
	t.Logf("read %d packets from %s", seen, name)
}

// readInBackground reads until the source fails, keeping up to a few
// packets for the test to look at.
func readInBackground(src Source) (<-chan Packet, <-chan error) {
	packets := make(chan Packet, 8)
	stopped := make(chan error, 1)
	go func() {
		for {
			pkt, err := src.Read()
			if err != nil {
				stopped <- err
				return
			}
			select {
			case packets <- pkt:
			default: // the test is not keeping up; drop it
			}
		}
	}()
	return packets, stopped
}

// collect checks every packet that arrives within readWindow and returns
// how many there were. It fails when the source stops on its own.
func collect(t *testing.T, packets <-chan Packet, stopped <-chan error) int {
	t.Helper()
	deadline := time.After(readWindow)
	seen := 0
	for {
		select {
		case pkt := <-packets:
			seen++
			checkPacket(t, seen, pkt)
		case err := <-stopped:
			t.Fatalf("read stopped before close: %v", err)
		case <-deadline:
			return seen
		}
	}
}

// expectEOF waits for the reader to stop and checks that it stopped
// because the source was closed.
func expectEOF(t *testing.T, stopped <-chan error) {
	t.Helper()
	select {
	case err := <-stopped:
		if !errors.Is(err, io.EOF) {
			t.Fatalf("read after close returned %v, want io.EOF", err)
		}
	case <-time.After(readWindow):
		t.Fatal("close did not unblock the read")
	}
}

func checkPacket(t *testing.T, n int, pkt Packet) {
	t.Helper()
	if len(pkt.Data) == 0 || len(pkt.Data) > snaplen {
		t.Fatalf("packet %d has %d bytes", n, len(pkt.Data))
	}
	if pkt.Length < len(pkt.Data) {
		t.Fatalf("packet %d is %d bytes on the wire but %d were captured", n, pkt.Length, len(pkt.Data))
	}
	if pkt.Time.IsZero() {
		t.Fatalf("packet %d has no capture time", n)
	}
	switch pkt.Link {
	case LinkEthernet, LinkNull, LinkRaw:
	default:
		t.Fatalf("packet %d has link type %v", n, pkt.Link)
	}
}
