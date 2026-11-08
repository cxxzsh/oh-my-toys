package packets

import "testing"

func TestDecodeTCP(t *testing.T) {
	for _, c := range []struct {
		name  string
		hdr   tcpHeader
		data  []byte
		info  string
		field string
		want  string
	}{
		{
			name: "SYN", hdr: tcpHeader{srcPort: 52114, dstPort: 443, flags: tcpFlagSYN},
			info: "52114 → 443 [SYN] Len=0",
		},
		{
			name: "push and acknowledgment", hdr: tcpHeader{srcPort: 51000, dstPort: 80, flags: tcpFlagPSH | tcpFlagACK},
			data: []byte("hello"), info: "51000 → 80 [PSH, ACK] Len=5",
		},
		{
			name: "no flags", hdr: tcpHeader{srcPort: 1, dstPort: 2},
			info: "1 → 2 [none] Len=0",
		},
		{
			name: "with options", hdr: tcpHeader{srcPort: 1, dstPort: 2, options: []byte{2, 4, 0x05, 0xb4}},
			data: []byte("data"), info: "1 → 2 [none] Len=4", field: fieldHeaderLen, want: "24",
		},
		{
			name: "acknowledgment number",
			hdr:  tcpHeader{srcPort: 1, dstPort: 2, seq: 7, ack: 123456, flags: tcpFlagACK, window: 65535},
			info: "1 → 2 [ACK] Len=0", field: "Acknowledgment number", want: "123456",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			flow := decode(v4Frame(numTCP, tcpSegment(c.hdr, c.data)))
			if flow.Info != c.info {
				t.Errorf("Info = %q, want %q", flow.Info, c.info)
			}
			if c.field != "" {
				if got := field(t, flow, layerTCP, c.field); got != c.want {
					t.Errorf("%s = %q, want %q", c.field, got, c.want)
				}
			}
		})
	}
}

func TestDecodeTCPWindow(t *testing.T) {
	flow := decode(v4Frame(numTCP, tcpSegment(tcpHeader{srcPort: 1, dstPort: 2, window: 64240}, nil)))
	if got := field(t, flow, layerTCP, "Window"); got != "64240" {
		t.Errorf("Window = %q", got)
	}
	if got := field(t, flow, layerTCP, "Flags"); got != "none" {
		t.Errorf("Flags = %q", got)
	}
}

func TestDecodeTCPBroken(t *testing.T) {
	short := tcpSegment(tcpHeader{srcPort: 1, dstPort: 2}, nil)[:10]
	badLength := tcpSegment(tcpHeader{srcPort: 1, dstPort: 2}, nil)
	badLength[12] = 0 // a header shorter than the fixed 20 bytes
	cutOptions := tcpSegment(tcpHeader{srcPort: 1, dstPort: 2}, nil)
	cutOptions[12] = 10 << nibbleBits // announces 40 bytes of header

	for _, c := range []struct {
		name  string
		seg   []byte
		info  string
		proto string
	}{
		{"truncated header", short, "[truncated] TCP header", protoIPv4},
		{"header length below the fixed size", badLength, "Malformed TCP header", protoIPv4},
		{"header length beyond the segment", cutOptions, "[truncated] TCP header", protoTCP},
	} {
		t.Run(c.name, func(t *testing.T) {
			flow := decode(v4Frame(numTCP, c.seg))
			if flow.Info != c.info {
				t.Errorf("Info = %q, want %q", flow.Info, c.info)
			}
			if flow.Proto != c.proto {
				t.Errorf("Proto = %q, want %q", flow.Proto, c.proto)
			}
		})
	}
}

func TestDecodeUDP(t *testing.T) {
	flow := decode(v4Frame(numUDP, udpSegment(1234, 9999, []byte("12345"))))
	if flow.Proto != protoUDP {
		t.Errorf("Proto = %q, want UDP", flow.Proto)
	}
	if flow.Info != "1234 → 9999 Len=5" {
		t.Errorf("Info = %q", flow.Info)
	}
	if flow.Host != testServerIP+":9999" {
		t.Errorf("Host = %q", flow.Host)
	}
	if got := field(t, flow, layerUDP, fieldLength); got != "13" {
		t.Errorf("Length = %q", got)
	}
	if got := field(t, flow, layerUDP, fieldSrcPort); got != "1234" {
		t.Errorf("Source port = %q", got)
	}
}

func TestDecodeUDPBroken(t *testing.T) {
	flow := decode(v4Frame(numUDP, udpSegment(1, 2, nil)[:6]))
	if flow.Info != "[truncated] UDP header" {
		t.Errorf("Info = %q", flow.Info)
	}
}

// icmpCase is one hand-made ICMP message and what it must decode to.
type icmpCase struct {
	name  string
	msg   []byte
	info  string
	typ   string
	code  string
	field string
	want  string
}

func TestDecodeICMP(t *testing.T) {
	for _, c := range []icmpCase{
		{
			name: "echo request", msg: icmpMessage(icmpEchoRequest, 0, echoBody(0x1234, 1)),
			info: "Echo (ping) request id=0x1234 seq=1",
			typ:  "Echo (ping) request", code: "0",
			field: "Identifier", want: "0x1234",
		},
		{
			name: "echo reply", msg: icmpMessage(icmpEchoReply, 0, echoBody(1, 2)),
			info: "Echo (ping) reply id=0x0001 seq=2",
			typ:  "Echo (ping) reply", code: "0",
		},
		{
			name: "port unreachable", msg: icmpMessage(icmpDestUnreachable, 3, make([]byte, 4)),
			info: "Destination unreachable", typ: "Destination unreachable",
			code: "Port unreachable (3)",
		},
		{
			name: "time exceeded", msg: icmpMessage(icmpTimeExceeded, 0, make([]byte, 4)),
			info: "Time-to-live exceeded", typ: "Time-to-live exceeded",
			code: "Time-to-live exceeded in transit (0)",
		},
		{
			name: "unknown type", msg: icmpMessage(42, 9, make([]byte, 4)),
			info: "ICMP type 42", typ: "ICMP type 42", code: "9",
		},
	} {
		t.Run(c.name, func(t *testing.T) { checkICMPCase(t, c) })
	}
}

// checkICMPCase decodes one message and compares what a consumer sees.
func checkICMPCase(t *testing.T, c icmpCase) {
	t.Helper()
	flow := decode(v4Frame(numICMP, c.msg))
	if flow.Proto != protoICMP {
		t.Errorf("Proto = %q, want ICMP", flow.Proto)
	}
	if flow.Info != c.info {
		t.Errorf("Info = %q, want %q", flow.Info, c.info)
	}
	if got := field(t, flow, layerICMP, fieldType); got != c.typ {
		t.Errorf("Type = %q, want %q", got, c.typ)
	}
	if got := field(t, flow, layerICMP, "Code"); got != c.code {
		t.Errorf("Code = %q, want %q", got, c.code)
	}
	if c.field != "" {
		if got := field(t, flow, layerICMP, c.field); got != c.want {
			t.Errorf("%s = %q, want %q", c.field, got, c.want)
		}
	}
}

func TestDecodeICMPTruncated(t *testing.T) {
	flow := decode(v4Frame(numICMP, []byte{icmpEchoRequest, 0}))
	if flow.Info != "[truncated] ICMP header" {
		t.Errorf("Info = %q", flow.Info)
	}
}

func TestDecodeICMPv6(t *testing.T) {
	echo := icmpMessage(icmpv6EchoRequest, 0, echoBody(1, 2))
	flow := decode(v6Frame(numICMPv6, echo))
	if flow.Proto != protoICMPv6 {
		t.Errorf("Proto = %q, want ICMPv6", flow.Proto)
	}
	if flow.Info != "Echo (ping) request id=0x0001 seq=2" {
		t.Errorf("Info = %q", flow.Info)
	}
	if got := field(t, flow, layerICMP6, fieldType); got != "Echo (ping) request" {
		t.Errorf("Type = %q", got)
	}

	target := mustIP(testServerV6)
	neighbor := icmpMessage(icmpv6NeighborSolicitation, 0, append(make([]byte, 4), target...))
	flow = decode(v6Frame(numICMPv6, neighbor))
	if flow.Info != "Neighbor solicitation for "+testServerV6 {
		t.Errorf("Info = %q", flow.Info)
	}
	if got := field(t, flow, layerICMP6, "Target address"); got != testServerV6 {
		t.Errorf("Target address = %q", got)
	}
}

func TestDecodeICMPv6Truncated(t *testing.T) {
	flow := decode(v6Frame(numICMPv6, []byte{icmpv6EchoRequest}))
	if flow.Info != "[truncated] ICMPv6 header" {
		t.Errorf("Info = %q", flow.Info)
	}
}
