package packets

import (
	"bytes"
	"slices"
	"testing"

	"github.com/cxxzsh/oh-my-toys/internal/platform/pcap"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

func TestDecodeFlowRow(t *testing.T) {
	frame := v4Frame(numTCP, tcpSegment(tcpHeader{srcPort: 52114, dstPort: 443, flags: tcpFlagSYN}, nil))
	p := pcap.Packet{Time: testTime, Link: pcap.LinkEthernet, Data: frame, Length: 74}
	flow := Decode(p)

	if flow.Source != tool.SourcePacket {
		t.Errorf("Source = %v, want packet", flow.Source)
	}
	if !flow.Started.Equal(testTime) {
		t.Errorf("Started = %v, want %v", flow.Started, testTime)
	}
	if flow.State != tool.FlowDone {
		t.Errorf("State = %v, want done", flow.State)
	}
	if flow.Size != 74 {
		t.Errorf("Size = %d, want 74 (the wire length)", flow.Size)
	}
	if flow.Proto != protoTCP {
		t.Errorf("Proto = %q, want %q", flow.Proto, protoTCP)
	}
	if flow.Host != testServerIP+":443" {
		t.Errorf("Host = %q, want the destination with its port", flow.Host)
	}
	if flow.Info != "52114 → 443 [SYN] Len=0" {
		t.Errorf("Info = %q", flow.Info)
	}
	if flow.Method != "" || flow.Path != "" || flow.Status != 0 {
		t.Errorf("a packet keeps no HTTP fields, got %q %q %d", flow.Method, flow.Path, flow.Status)
	}
	if flow.Packet == nil {
		t.Fatal("no packet detail")
	}
	if !bytes.Equal(flow.Packet.Raw, frame) {
		t.Errorf("Raw = %x, want the captured bytes", flow.Packet.Raw)
	}
	frame[0] = 0xff
	if flow.Packet.Raw[0] == 0xff {
		t.Error("Raw aliases the capture buffer")
	}
}

func TestDecodeCutsRawAtMaxBody(t *testing.T) {
	data := bytes.Repeat([]byte{0x45}, tool.MaxBody+16)
	flow := decodeLink(pcap.LinkRaw, data)
	if len(flow.Packet.Raw) != tool.MaxBody {
		t.Errorf("Raw is %d bytes, want %d", len(flow.Packet.Raw), tool.MaxBody)
	}
}

// linkCase is one packet and the flow it must decode to.
type linkCase struct {
	name   string
	link   pcap.LinkType
	data   []byte
	proto  string
	info   string
	layers []string
}

func TestDecodeLinkTypes(t *testing.T) {
	seg := tcpSegment(tcpHeader{srcPort: 50000, dstPort: 80, flags: tcpFlagACK}, []byte("hi"))
	v4 := ipv4Packet(ipv4Header{src: testClientIP, dst: testServerIP, proto: numTCP}, seg)
	v6 := ipv6Packet(testClientV6, testServerV6, numTCP, seg)
	arp := ethFrame("ff:ff:ff:ff:ff:ff", testClientMAC, etherTypeARP,
		arpPacket(arpOpRequest, testClientMAC, testClientIP, "00:00:00:00:00:00", "192.168.1.1"))
	vlan := ethFrame(testRouterMAC, testClientMAC, etherTypeVLAN, append(vlanTag(5, 42, etherTypeIPv4), v4...))
	qinq := ethFrame(testRouterMAC, testClientMAC, etherTypeQinQ,
		append(vlanTag(0, 7, etherTypeVLAN), append(vlanTag(1, 8, etherTypeIPv4), v4...)...))

	for _, c := range []linkCase{
		{
			name: "ethernet IPv4", link: pcap.LinkEthernet,
			data:  ethFrame(testRouterMAC, testClientMAC, etherTypeIPv4, v4),
			proto: protoTCP, layers: []string{layerEthernet, layerIPv4, layerTCP},
		},
		{
			name: "ethernet IPv6", link: pcap.LinkEthernet,
			data:  ethFrame(testRouterMAC, testClientMAC, etherTypeIPv6, v6),
			proto: protoTCP, layers: []string{layerEthernet, layerIPv6, layerTCP},
		},
		{
			name: "raw IPv4", link: pcap.LinkRaw, data: v4, proto: protoTCP,
			layers: []string{layerIPv4, layerTCP},
		},
		{
			name: "raw IPv6", link: pcap.LinkRaw, data: v6, proto: protoTCP,
			layers: []string{layerIPv6, layerTCP},
		},
		{
			name: "null family 2 little endian", link: pcap.LinkNull,
			data:  append([]byte{2, 0, 0, 0}, v4...),
			proto: protoTCP, layers: []string{layerLoopback, layerIPv4, layerTCP},
		},
		{
			name: "null family 2 big endian", link: pcap.LinkNull,
			data:  append([]byte{0, 0, 0, 2}, v4...),
			proto: protoTCP, layers: []string{layerLoopback, layerIPv4, layerTCP},
		},
		{
			name: "null family 30", link: pcap.LinkNull,
			data:  append([]byte{30, 0, 0, 0}, v6...),
			proto: protoTCP, layers: []string{layerLoopback, layerIPv6, layerTCP},
		},
		{
			name: "null family 28", link: pcap.LinkNull,
			data:  append([]byte{28, 0, 0, 0}, v6...),
			proto: protoTCP, layers: []string{layerLoopback, layerIPv6, layerTCP},
		},
		{
			name: "null family 24", link: pcap.LinkNull,
			data:  append([]byte{24, 0, 0, 0}, v6...),
			proto: protoTCP, layers: []string{layerLoopback, layerIPv6, layerTCP},
		},
		{
			name: "null unknown family", link: pcap.LinkNull,
			data:  append([]byte{99, 0, 0, 0}, v4...),
			proto: protoUnknown, info: "Loopback address family 99", layers: []string{layerLoopback},
		},
		{
			name: "ARP request", link: pcap.LinkEthernet, data: arp, proto: protoARP,
			info: "Who has 192.168.1.1? Tell 192.168.1.20", layers: []string{layerEthernet, layerARP},
		},
		{
			name: "one VLAN tag", link: pcap.LinkEthernet, data: vlan, proto: protoTCP,
			layers: []string{layerEthernet, layerVLAN, layerIPv4, layerTCP},
		},
		{
			name: "two stacked tags", link: pcap.LinkEthernet, data: qinq, proto: protoTCP,
			layers: []string{layerEthernet, layerVLAN, layerVLAN, layerIPv4, layerTCP},
		},
		{
			name: "unknown link type", link: pcap.LinkType(9), data: v4, proto: protoUnknown,
			info: "Unknown link type 9",
		},
		{
			name: "truncated Ethernet", link: pcap.LinkEthernet, data: make([]byte, 10),
			proto: protoUnknown, info: "[truncated] Ethernet header",
		},
		{
			name: "empty raw packet", link: pcap.LinkRaw, data: nil,
			proto: protoUnknown, info: "[truncated] IP header",
		},
		{
			name: "raw IP version 7", link: pcap.LinkRaw, data: []byte{0x70, 1, 2, 3},
			proto: protoUnknown, info: "IP version 7",
		},
		{
			name: "unknown EtherType", link: pcap.LinkEthernet,
			data:  ethFrame(testRouterMAC, testClientMAC, 0x88cc, []byte{1, 2, 3}),
			proto: protoEthernet, info: "Ethernet type 0x88cc", layers: []string{layerEthernet},
		},
		{
			name: "truncated VLAN tag", link: pcap.LinkEthernet,
			data:  ethFrame(testRouterMAC, testClientMAC, etherTypeVLAN, []byte{0, 1, 0x08}),
			proto: protoEthernet, info: "[truncated] 802.1Q tag", layers: []string{layerEthernet},
		},
	} {
		t.Run(c.name, func(t *testing.T) { checkLinkCase(t, c) })
	}
}

// checkLinkCase decodes one packet and compares what a consumer sees.
func checkLinkCase(t *testing.T, c linkCase) {
	t.Helper()
	flow := decodeLink(c.link, c.data)
	if flow.Proto != c.proto {
		t.Errorf("Proto = %q, want %q", flow.Proto, c.proto)
	}
	if c.info != "" && flow.Info != c.info {
		t.Errorf("Info = %q, want %q", flow.Info, c.info)
	}
	if len(c.layers) > 0 {
		if got := layerNames(flow); !slices.Equal(got, c.layers) {
			t.Errorf("layers = %v, want %v", got, c.layers)
		}
	}
}

func TestDecodeEthernetFields(t *testing.T) {
	frame := ethFrame(testRouterMAC, testClientMAC, etherTypeIPv4,
		ipv4Packet(ipv4Header{src: testClientIP, dst: testServerIP, proto: numUDP}, nil))
	flow := decode(frame)
	if got := field(t, flow, layerEthernet, fieldSource); got != testClientMAC {
		t.Errorf("Source = %q, want %q", got, testClientMAC)
	}
	if got := field(t, flow, layerEthernet, fieldDestination); got != testRouterMAC {
		t.Errorf("Destination = %q, want %q", got, testRouterMAC)
	}
	if got := field(t, flow, layerEthernet, fieldType); got != "IPv4 (0x0800)" {
		t.Errorf("Type = %q", got)
	}
}

func TestDecodeVLANFields(t *testing.T) {
	frame := ethFrame(testRouterMAC, testClientMAC, etherTypeVLAN,
		append(vlanTag(3, 100, etherTypeARP),
			arpPacket(arpOpReply, "00:11:22:33:44:55", "192.168.1.1", testClientMAC, testClientIP)...))
	flow := decode(frame)
	if got := field(t, flow, layerVLAN, "Priority"); got != "3" {
		t.Errorf("Priority = %q", got)
	}
	if got := field(t, flow, layerVLAN, "ID"); got != "100" {
		t.Errorf("ID = %q", got)
	}
	if got := field(t, flow, layerVLAN, "Drop eligible"); got != "No" {
		t.Errorf("Drop eligible = %q", got)
	}
}

func TestDecodeARPReply(t *testing.T) {
	frame := ethFrame(testClientMAC, "00:11:22:33:44:55", etherTypeARP,
		arpPacket(arpOpReply, "00:11:22:33:44:55", "192.168.1.1", testClientMAC, testClientIP))
	flow := decode(frame)
	if flow.Info != "192.168.1.1 is at 00:11:22:33:44:55" {
		t.Errorf("Info = %q", flow.Info)
	}
	if flow.Host != testClientIP {
		t.Errorf("Host = %q, want the target address of the reply", flow.Host)
	}
	if got := field(t, flow, layerARP, "Operation"); got != "Reply (2)" {
		t.Errorf("Operation = %q", got)
	}
	if got := field(t, flow, layerARP, "Sender IP"); got != "192.168.1.1" {
		t.Errorf("Sender IP = %q", got)
	}
	if got := field(t, flow, layerARP, "Target MAC"); got != testClientMAC {
		t.Errorf("Target MAC = %q", got)
	}
}

func TestDecodeTruncatedARP(t *testing.T) {
	frame := ethFrame(testRouterMAC, testClientMAC, etherTypeARP,
		arpPacket(arpOpRequest, testClientMAC, testClientIP, "00:00:00:00:00:00", "192.168.1.1")[:20])
	flow := decode(frame)
	if flow.Info != "[truncated] ARP addresses" {
		t.Errorf("Info = %q", flow.Info)
	}
	if flow.Proto != protoARP {
		t.Errorf("Proto = %q, want ARP", flow.Proto)
	}
}

func TestDecodeLoopbackFamilyText(t *testing.T) {
	v4 := ipv4Packet(ipv4Header{src: testClientIP, dst: testServerIP, proto: numUDP}, nil)
	flow := decodeLink(pcap.LinkNull, append([]byte{2, 0, 0, 0}, v4...))
	if got := field(t, flow, layerLoopback, "Family"); got != "IPv4 (2)" {
		t.Errorf("Family = %q", got)
	}
}
