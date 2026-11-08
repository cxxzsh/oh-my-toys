package packets

import "testing"

// ipv4Case is one hand-made IPv4 packet and what it must decode to.
type ipv4Case struct {
	name  string
	hdr   ipv4Header
	proto string
	info  string
	layer string // the layer after IPv4 that must follow, empty for none
	field string
	want  string
}

func TestDecodeIPv4(t *testing.T) {
	for _, c := range []ipv4Case{
		{
			name:  "TCP",
			hdr:   ipv4Header{src: testClientIP, dst: testServerIP, proto: numTCP, ttl: 64},
			proto: protoTCP, layer: layerTCP,
		},
		{
			name:  "with options",
			hdr:   ipv4Header{src: testClientIP, dst: testServerIP, proto: numTCP, options: []byte{1, 1, 1, 0}},
			proto: protoTCP, layer: layerTCP, field: fieldHeaderLen, want: "24",
		},
		{
			name:  "first fragment",
			hdr:   ipv4Header{src: testClientIP, dst: testServerIP, proto: numTCP, flags: ipv4FlagMF},
			proto: protoTCP, layer: layerTCP, field: "Flags", want: "MF",
		},
		{
			name:  "later fragment",
			hdr:   ipv4Header{src: testClientIP, dst: testServerIP, proto: numTCP, flags: ipv4FlagMF | 185},
			proto: protoIPv4, info: "IPv4 fragment, offset 1480",
			field: fieldDestination, want: testServerIP,
		},
		{
			name:  "unknown protocol",
			hdr:   ipv4Header{src: testClientIP, dst: testServerIP, proto: 47},
			proto: protoIPv4, info: "IP protocol 47", field: "Protocol", want: "47",
		},
		{
			name:  "don't fragment",
			hdr:   ipv4Header{src: testClientIP, dst: testServerIP, proto: numTCP, flags: ipv4FlagDF},
			proto: protoTCP, layer: layerTCP, field: "Flags", want: "DF",
		},
	} {
		t.Run(c.name, func(t *testing.T) { checkIPv4Case(t, c) })
	}
}

// checkIPv4Case decodes one case and compares what a consumer sees.
func checkIPv4Case(t *testing.T, c ipv4Case) {
	t.Helper()
	seg := tcpSegment(tcpHeader{srcPort: 51000, dstPort: 443, flags: tcpFlagSYN}, nil)
	flow := decode(ethFrame(testRouterMAC, testClientMAC, etherTypeIPv4, ipv4Packet(c.hdr, seg)))
	if flow.Proto != c.proto {
		t.Errorf("Proto = %q, want %q", flow.Proto, c.proto)
	}
	if c.info != "" && flow.Info != c.info {
		t.Errorf("Info = %q, want %q", flow.Info, c.info)
	}
	if c.layer != "" && !hasLayer(flow, c.layer) {
		t.Errorf("layers = %v, want %q", layerNames(flow), c.layer)
	}
	if c.field != "" {
		if got := field(t, flow, layerIPv4, c.field); got != c.want {
			t.Errorf("%s = %q, want %q", c.field, got, c.want)
		}
	}
}

func TestDecodeIPv4FragmentHasNoTransport(t *testing.T) {
	seg := tcpSegment(tcpHeader{srcPort: 1, dstPort: 2}, nil)
	frame := ethFrame(testRouterMAC, testClientMAC, etherTypeIPv4,
		ipv4Packet(ipv4Header{src: testClientIP, dst: testServerIP, proto: numTCP, flags: 100}, seg))
	flow := decode(frame)
	if hasLayer(flow, layerTCP) {
		t.Fatalf("a later fragment decodes the bytes after its header as TCP: %v", layerNames(flow))
	}
	if flow.Host != testServerIP {
		t.Errorf("Host = %q, want the destination address", flow.Host)
	}
}

func TestDecodeIPv4TrimsPadding(t *testing.T) {
	udp := udpSegment(1000, 2000, []byte("abcdefgh"))
	hdr := ipv4Header{
		src: testClientIP, dst: testServerIP, proto: numUDP,
		totalLen: ipv4HeaderLen + udpHeaderLen + 4,
	}
	flow := decode(ethFrame(testRouterMAC, testClientMAC, etherTypeIPv4, ipv4Packet(hdr, udp)))
	if flow.Info != "1000 → 2000 Len=4" {
		t.Errorf("Info = %q, want the length the header announced", flow.Info)
	}
}

func TestDecodeIPv4SnappedShort(t *testing.T) {
	seg := tcpSegment(tcpHeader{srcPort: 7, dstPort: 8, flags: tcpFlagACK}, []byte("hello"))
	hdr := ipv4Header{src: testClientIP, dst: testServerIP, proto: numTCP, totalLen: 1400}
	flow := decode(ethFrame(testRouterMAC, testClientMAC, etherTypeIPv4, ipv4Packet(hdr, seg)))
	if flow.Proto != protoTCP || flow.Info != "7 → 8 [ACK] Len=5" {
		t.Errorf("Proto = %q, Info = %q; a snapped capture should decode what is there", flow.Proto, flow.Info)
	}
}

func TestDecodeIPv4Broken(t *testing.T) {
	short := ethFrame(testRouterMAC, testClientMAC, etherTypeIPv4, make([]byte, 10))
	badIHL := ipv4Packet(ipv4Header{src: testClientIP, dst: testServerIP, proto: numTCP}, nil)
	badIHL[0] = 0x44
	badVersion := ipv4Packet(ipv4Header{src: testClientIP, dst: testServerIP, proto: numTCP}, nil)
	badVersion[0] = 0x65
	broken := func(packet []byte) []byte {
		return ethFrame(testRouterMAC, testClientMAC, etherTypeIPv4, packet)
	}

	for _, c := range []struct {
		name  string
		frame []byte
		info  string
		proto string
	}{
		{"truncated header", short, "[truncated] IPv4 header", protoEthernet},
		{"header length below the fixed size", broken(badIHL), "Malformed IPv4 header", protoEthernet},
		{"wrong version", broken(badVersion), "Malformed IPv4 header", protoEthernet},
	} {
		t.Run(c.name, func(t *testing.T) {
			flow := decode(c.frame)
			if flow.Info != c.info {
				t.Errorf("Info = %q, want %q", flow.Info, c.info)
			}
			if flow.Proto != c.proto {
				t.Errorf("Proto = %q, want %q", flow.Proto, c.proto)
			}
		})
	}
}

func TestDecodeIPv6(t *testing.T) {
	seg := tcpSegment(tcpHeader{srcPort: 51000, dstPort: 443, flags: tcpFlagACK}, []byte("x"))
	for _, c := range []struct {
		name    string
		next    byte
		payload []byte
		proto   string
		info    string
		hasTCP  bool
	}{
		{
			name: "TCP", next: numTCP, payload: seg, proto: protoTCP, hasTCP: true,
		},
		{
			name: "hop by hop then TCP", next: extHopByHop,
			payload: append(extHeader(numTCP, 0), seg...), proto: protoTCP, hasTCP: true,
		},
		{
			name: "routing then destination options then TCP", next: extRouting,
			payload: append(append(extHeader(extDestOpts, 1), extHeader(numTCP, 0)...), seg...),
			proto:   protoTCP, hasTCP: true,
		},
		{
			name: "first fragment", next: extFragment,
			payload: append(fragmentHeader(numTCP, 0), seg...), proto: protoTCP, hasTCP: true,
		},
		{
			name: "later fragment", next: extFragment,
			payload: append(fragmentHeader(numTCP, 1), seg...), proto: protoIPv6, info: "IPv6 fragment, offset 8",
		},
		{
			name: "truncated extension header", next: extHopByHop, payload: []byte{numTCP},
			proto: protoIPv6, info: "[truncated] IPv6 extension header",
		},
		{
			name: "extension header longer than the payload", next: extHopByHop,
			payload: []byte{numTCP, 40, 0, 0}, proto: protoIPv6, info: "[truncated] IPv6 extension header",
		},
		{
			name: "extension headers that never end", next: extHopByHop,
			payload: make([]byte, (maxExtensionHeaders+1)*extHeaderUnit),
			proto:   protoIPv6, info: "Malformed IPv6 extension headers",
		},
		{
			name: "no next header", next: numNoNext, payload: nil,
			proto: protoIPv6, info: "No next header",
		},
		{
			name: "unknown protocol", next: 47, payload: []byte{1, 2, 3},
			proto: protoIPv6, info: "IP protocol 47",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			flow := decode(v6Frame(c.next, c.payload))
			if flow.Proto != c.proto {
				t.Errorf("Proto = %q, want %q", flow.Proto, c.proto)
			}
			if c.info != "" && flow.Info != c.info {
				t.Errorf("Info = %q, want %q", flow.Info, c.info)
			}
			if hasLayer(flow, layerTCP) != c.hasTCP {
				t.Errorf("layers = %v, TCP expected: %v", layerNames(flow), c.hasTCP)
			}
		})
	}
}

func TestDecodeIPv6Fields(t *testing.T) {
	flow := decode(v6Frame(numTCP, tcpSegment(tcpHeader{srcPort: 1000, dstPort: 22}, nil)))
	if got := field(t, flow, layerIPv6, fieldSource); got != testClientV6 {
		t.Errorf("Source = %q, want %q", got, testClientV6)
	}
	if got := field(t, flow, layerIPv6, fieldDestination); got != testServerV6 {
		t.Errorf("Destination = %q, want %q", got, testServerV6)
	}
	if got := field(t, flow, layerIPv6, "Next header"); got != "TCP (6)" {
		t.Errorf("Next header = %q", got)
	}
	if got := field(t, flow, layerIPv6, "Hop limit"); got != "64" {
		t.Errorf("Hop limit = %q", got)
	}
	if flow.Host != "["+testServerV6+"]:22" {
		t.Errorf("Host = %q", flow.Host)
	}
}

func TestDecodeIPv6Broken(t *testing.T) {
	short := ethFrame(testRouterMAC, testClientMAC, etherTypeIPv6, make([]byte, 20))
	badVersion := ipv6Packet(testClientV6, testServerV6, numTCP, nil)
	badVersion[0] = 0x45

	for _, c := range []struct {
		name  string
		frame []byte
		info  string
	}{
		{"truncated header", short, "[truncated] IPv6 header"},
		{"wrong version", ethFrame(testRouterMAC, testClientMAC, etherTypeIPv6, badVersion), "Malformed IPv6 header"},
	} {
		t.Run(c.name, func(t *testing.T) {
			flow := decode(c.frame)
			if flow.Info != c.info {
				t.Errorf("Info = %q, want %q", flow.Info, c.info)
			}
		})
	}
}
