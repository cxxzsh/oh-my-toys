//go:build darwin

package pcap

import (
	"encoding/binary"
	"errors"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unsafe"
)

// readWindow is how long a live read is given before the test closes the
// source.
const readWindow = 2 * time.Second

func TestStructLayouts(t *testing.T) {
	// The numbers the C headers give on a 64-bit darwin. A Go struct that
	// drifts from them makes every field of the device list and of every
	// packet header land in the wrong place.
	for _, c := range []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"pcap_pkthdr", unsafe.Sizeof(pcapHeader{}), 24},
		{"pcap_pkthdr.caplen", unsafe.Offsetof(pcapHeader{}.caplen), 16},
		{"pcap_pkthdr.len", unsafe.Offsetof(pcapHeader{}.length), 20},
		{"pcap_if", unsafe.Sizeof(cIf{}), 40},
		{"pcap_if.flags", unsafe.Offsetof(cIf{}.flags), 32},
		{"pcap_addr", unsafe.Sizeof(cAddr{}), 40},
	} {
		if c.got != c.want {
			t.Errorf("%s is %d bytes at the wrong place, want %d", c.name, c.got, c.want)
		}
	}
}

// TestWalkDevices walks a buffer laid out the way libpcap lays its device
// list out, so the field offsets are tested on the real thing rather than
// on Go values of the same type.
func TestWalkDevices(t *testing.T) {
	const (
		sockInSize = 16 // sockaddr_in
		bufSize    = 1024
	)
	buf := make([]byte, bufSize)
	base := uintptr(unsafe.Pointer(&buf[0]))
	ifSize := int(unsafe.Sizeof(cIf{}))
	addrSize := int(unsafe.Sizeof(cAddr{}))

	// The layout: two pcap_if, one pcap_addr, two sockaddr_in, then the
	// strings the structs point at.
	dev0, dev1 := 0, ifSize
	addr := 2 * ifSize
	in4 := addr + addrSize
	in4Loop := in4 + sockInSize
	name0 := in4Loop + sockInSize
	name1 := name0 + 4
	desc1 := name1 + 4

	copy(buf[name0:], "en0\x00")
	copy(buf[name1:], "lo0\x00")
	copy(buf[desc1:], "loopback\x00")

	put32 := func(at int, v uint32) { binary.LittleEndian.PutUint32(buf[at:], v) }
	putPtr := func(at, target int) { binary.LittleEndian.PutUint64(buf[at:], uint64(base)+uint64(target)) }
	putNil := func(at int) { binary.LittleEndian.PutUint64(buf[at:], 0) }

	// The first device: up, no addresses, no description, and the second
	// device after it.
	putPtr(dev0, dev1)
	putPtr(dev0+8, name0)
	putNil(dev0 + 16)
	putNil(dev0 + 24)
	put32(dev0+32, ifUp)
	// The second: no flags at all, as a libpcap older than 1.9 reports,
	// with the loopback address as its only address.
	putNil(dev1)
	putPtr(dev1+8, name1)
	putPtr(dev1+16, desc1)
	putPtr(dev1+24, addr)
	put32(dev1+32, 0)
	// pcap_addr -> 127.0.0.1
	putNil(addr)
	putPtr(addr+8, in4Loop)
	putNil(addr + 16)
	putNil(addr + 24)
	putNil(addr + 32)
	// sockaddr_in for 10.0.0.5, which no device points at any more.
	for _, at := range []int{in4, in4Loop} {
		buf[at] = sockInSize
		buf[at+1] = afInet
	}
	buf[in4+4], buf[in4+7] = 10, 5
	buf[in4Loop+4], buf[in4Loop+7] = loopbackV4, 1

	got := walkDevices((*cIf)(unsafe.Pointer(&buf[0])))
	want := []Interface{
		{Name: "en0", Up: true},
		{Name: "lo0", Description: "loopback", Loopback: true},
	}
	if len(got) != len(want) {
		t.Fatalf("walked %d devices, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("device %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// fakeNext scripts pcap_next_ex: each call returns the next step, and the
// last step repeats.
func fakeNext(steps []int32, header *pcapHeader, data *byte) func(unsafe.Pointer, **pcapHeader, **byte) int32 {
	calls := 0
	return func(_ unsafe.Pointer, h **pcapHeader, d **byte) int32 {
		ret := steps[min(calls, len(steps)-1)]
		calls++
		if ret == nextOK {
			*h = header
			*d = data
		}
		return ret
	}
}

func TestReadWaitsThroughTimeouts(t *testing.T) {
	data := []byte{0xaa, 0xbb}
	header := &pcapHeader{
		ts:     pcapTimeval{sec: 1700, usec: 250000},
		caplen: uint32(len(data)),
		length: 64,
	}
	src := &source{
		api:  &darwinFuncs{nextEx: fakeNext([]int32{nextTimeout, nextTimeout, nextOK}, header, &data[0])},
		name: "en0",
		link: LinkRaw,
	}

	pkt, err := src.Read()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if pkt.Time.Unix() != 1700 || pkt.Time.Nanosecond() != 250*int(time.Millisecond) {
		t.Errorf("packet time = %v, want the timestamp libpcap gave", pkt.Time)
	}
	if pkt.Length != 64 || pkt.Link != LinkRaw || !slices.Equal(pkt.Data, data) {
		t.Errorf("packet = %+v, want 64 bytes of %v as %v", pkt, data, LinkRaw)
	}
	// The bytes belong to the packet, not to libpcap's buffer.
	data[0] = 0x11
	if pkt.Data[0] != 0xaa {
		t.Error("the packet's bytes changed with the capture buffer")
	}
}

func TestReadEnds(t *testing.T) {
	for _, c := range []struct {
		name string
		step int32
		want error
		text string
	}{
		{"breakloop", nextClosed, io.EOF, ""},
		{"failure", -1, nil, "-1"},
	} {
		src := &source{
			api:  &darwinFuncs{nextEx: fakeNext([]int32{c.step}, nil, nil)},
			name: "en0",
		}
		_, err := src.Read()
		if c.want != nil {
			if !errors.Is(err, c.want) {
				t.Errorf("%s: read returned %v, want %v", c.name, err, c.want)
			}
			continue
		}
		if err == nil || errors.Is(err, io.EOF) || !strings.Contains(err.Error(), c.text) {
			t.Errorf("%s: read returned %v, want an error mentioning %q", c.name, err, c.text)
		}
	}
}

func TestReadRejectsAnOversizedPacket(t *testing.T) {
	data := make([]byte, 1)
	header := &pcapHeader{caplen: snaplen + 1, length: snaplen + 1}
	src := &source{
		api:  &darwinFuncs{nextEx: fakeNext([]int32{nextOK}, header, &data[0])},
		name: "en0",
	}
	if _, err := src.Read(); err == nil {
		t.Error("a packet larger than the snapshot length was accepted")
	}
}

// TestCloseUnblocksRead holds a read inside pcap_next_ex and checks that
// Close wakes it, frees the handle once, and that a later read reports
// io.EOF instead of touching libpcap again.
func TestCloseUnblocksRead(t *testing.T) {
	entered := make(chan struct{}, 1)
	unblock := make(chan struct{})
	closes := 0
	src := &source{
		api: &darwinFuncs{
			nextEx: func(unsafe.Pointer, **pcapHeader, **byte) int32 {
				entered <- struct{}{}
				<-unblock
				return nextClosed
			},
			breakLoop: func(unsafe.Pointer) { close(unblock) },
			closeFn:   func(unsafe.Pointer) { closes++ },
		},
		name: "en0",
	}

	read := make(chan error, 1)
	go func() {
		_, err := src.Read()
		read <- err
	}()
	<-entered

	if err := src.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	select {
	case err := <-read:
		if !errors.Is(err, io.EOF) {
			t.Fatalf("read after close returned %v, want io.EOF", err)
		}
	case <-time.After(readWindow):
		t.Fatal("close did not unblock the read")
	}
	if closes != 1 {
		t.Fatalf("libpcap was closed %d times, want once", closes)
	}
	if _, err := src.Read(); !errors.Is(err, io.EOF) {
		t.Errorf("read after close returned %v, want io.EOF", err)
	}
	if err := src.Close(); err != nil || closes != 1 {
		t.Errorf("closing again returned %v and closed %d times, want nil and once", err, closes)
	}
}

func TestRouteCommandInterface(t *testing.T) {
	for _, c := range []struct {
		name string
		out  string
		want string
	}{
		{
			name: "default route",
			out: "   route to: default\ndestination: default\n" +
				"    gateway: 192.168.1.1\n  interface: en0\n     flags: <UP,GATEWAY,DONE>\n",
			want: "en0",
		},
		{"no route", "route: writing to routing socket: not in table\n", ""},
		{"empty", "", ""},
	} {
		if got := routeCommandInterface([]byte(c.out)); got != c.want {
			t.Errorf("%s: interface = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestAppleQuote(t *testing.T) {
	for _, c := range []struct {
		name string
		in   string
		want string
	}{
		{"plain", "chmod o+r /dev/bpf*", "chmod o+r /dev/bpf*"},
		{"quote", `say "hi"`, `say \"hi\"`},
		{"backslash", `a\b`, `a\\b`},
		{"both", `a\"b`, `a\\\"b`},
	} {
		if got := appleQuote(c.in); got != c.want {
			t.Errorf("%s: appleQuote(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestGrantScript(t *testing.T) {
	const reason = "Oh My Toys needs to read the capture devices to record packets."
	for _, c := range []struct {
		name    string
		command string
		reason  string
		want    string
	}{
		{
			name:    "the real grant",
			command: bpfGrantCommand,
			reason:  reason,
			want: `do shell script "chgrp admin /dev/bpf* && chmod g+rw /dev/bpf*"` +
				` with administrator privileges with prompt "` + reason + `"`,
		},
		{
			name:    "quotes and backslashes in both strings",
			command: `say "hi"`,
			reason:  `a "quoted" \ reason`,
			want: `do shell script "say \"hi\"" with administrator privileges` +
				` with prompt "a \"quoted\" \\ reason"`,
		},
	} {
		if got := grantScript(c.command, c.reason); got != c.want {
			t.Errorf("%s: script = %q, want %q", c.name, got, c.want)
		}
	}
}

// TestBPFGrantCommand pins what the prompt runs: the devices go to the
// administrators' group, and never to every local user, who would then
// read all of this machine's traffic.
func TestBPFGrantCommand(t *testing.T) {
	if !strings.Contains(bpfGrantCommand, "chgrp "+adminGroup+" ") {
		t.Errorf("grant command %q does not hand the devices to the %q group", bpfGrantCommand, adminGroup)
	}
	if !strings.Contains(bpfGrantCommand, "chmod g+rw ") {
		t.Errorf("grant command %q does not give the group read-write, the access libpcap prefers", bpfGrantCommand)
	}
	if !strings.Contains(bpfGrantCommand, "&&") {
		t.Errorf("grant command %q runs chmod even when chgrp failed", bpfGrantCommand)
	}
	for _, tooWide := range []string{"o+r", "o+rw", "a+rw", "777"} {
		if strings.Contains(bpfGrantCommand, tooWide) {
			t.Errorf("grant command %q gives every user access (%q)", bpfGrantCommand, tooWide)
		}
	}
}

// TestProgramPaths keeps route and osascript away from PATH, where another
// program could answer for them.
func TestProgramPaths(t *testing.T) {
	for _, path := range []string{routeCommand, osascriptCommand} {
		if !filepath.IsAbs(path) {
			t.Errorf("%q is found through PATH, where another program could answer", path)
		}
	}
}

func TestCancelled(t *testing.T) {
	for _, c := range []struct {
		name string
		out  string
		want bool
	}{
		{"user cancelled", "0:1: execution error: User canceled. (-128)\n", true},
		{"authorisation failed", "0:1: execution error: Authentication was not completed. (-60009)\n", false},
		{"empty", "", false},
	} {
		if got := cancelled([]byte(c.out)); got != c.want {
			t.Errorf("%s: cancelled(%q) = %v, want %v", c.name, c.out, got, c.want)
		}
	}
}
