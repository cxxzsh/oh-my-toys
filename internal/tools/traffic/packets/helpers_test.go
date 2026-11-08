package packets

import (
	"bytes"
	"encoding/binary"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/platform/pcap"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// Fixed endpoints the hand-made packets talk between.
const (
	testClientMAC = "aa:bb:cc:dd:ee:ff"
	testRouterMAC = "00:11:22:33:44:55"
	testClientIP  = "192.168.1.20"
	testServerIP  = "93.184.216.34"
	testClientV6  = "2001:db8::1"
	testServerV6  = "2001:db8::2"
)

// testTime is the capture time the tests expect to see on a flow.
var testTime = time.Date(2026, 11, 8, 12, 30, 15, 0, time.UTC)

// decode decodes an Ethernet frame with the time and length the tests use.
func decode(data []byte) tool.Flow {
	return Decode(pcap.Packet{Time: testTime, Link: pcap.LinkEthernet, Data: data, Length: len(data)})
}

// decodeLink decodes bytes that start with the given link layer.
func decodeLink(link pcap.LinkType, data []byte) tool.Flow {
	return Decode(pcap.Packet{Time: testTime, Link: link, Data: data, Length: len(data)})
}

func mustIP(s string) []byte {
	ip := net.ParseIP(s)
	if ip == nil {
		panic("bad test IP " + s)
	}
	if v4 := ip.To4(); v4 != nil {
		return v4
	}
	return ip.To16()
}

func mustMAC(s string) []byte {
	m, err := net.ParseMAC(s)
	if err != nil {
		panic("bad test MAC " + s)
	}
	return m
}

func u16(v uint16) []byte {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, v)
	return b
}

func u24(v int) []byte {
	return []byte{byte(v >> 16), byte(v >> 8), byte(v)}
}

func u32(v uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, v)
	return b
}

// ethFrame is an Ethernet frame with one payload of the given EtherType.
func ethFrame(dst, src string, typ uint16, payload []byte) []byte {
	frame := append(append([]byte{}, mustMAC(dst)...), mustMAC(src)...)
	frame = append(frame, byte(typ>>8), byte(typ))
	return append(frame, payload...)
}

// vlanTag is one 802.1Q tag: priority and drop bit, the ID, then the type
// of what follows.
func vlanTag(priority byte, id, typ uint16) []byte {
	tci := uint16(priority)<<vlanPriorityBit | id
	return append(u16(tci), u16(typ)...)
}

// ipv4Header is the part of an IPv4 header the tests vary.
type ipv4Header struct {
	src, dst string
	proto    byte
	ttl      byte
	flags    uint16 // the flags and fragment offset field
	options  []byte
	totalLen int // 0: the real length
}

func ipv4Packet(h ipv4Header, payload []byte) []byte {
	hl := ipv4HeaderLen + len(h.options)
	total := h.totalLen
	if total == 0 {
		total = hl + len(payload)
	}
	pkt := make([]byte, hl, hl+len(payload))
	pkt[0] = ipVersion4<<nibbleBits | byte(hl/ihlUnit)
	binary.BigEndian.PutUint16(pkt[2:4], uint16(total))
	binary.BigEndian.PutUint16(pkt[6:8], h.flags)
	pkt[8] = h.ttl
	pkt[9] = h.proto
	copy(pkt[12:16], mustIP(h.src))
	copy(pkt[16:20], mustIP(h.dst))
	copy(pkt[20:], h.options)
	return append(pkt, payload...)
}

func ipv6Packet(src, dst string, next byte, payload []byte) []byte {
	pkt := make([]byte, ipv6HeaderLen, ipv6HeaderLen+len(payload))
	pkt[0] = ipVersion6 << nibbleBits
	binary.BigEndian.PutUint16(pkt[4:6], uint16(len(payload)))
	pkt[6] = next
	pkt[7] = 64
	copy(pkt[8:24], mustIP(src))
	copy(pkt[24:40], mustIP(dst))
	return append(pkt, payload...)
}

// extHeader is an IPv6 extension header of (units+1)*8 bytes.
func extHeader(next, units byte) []byte {
	h := make([]byte, (int(units)+1)*extHeaderUnit)
	h[0], h[1] = next, units
	return h
}

// fragmentHeader is an IPv6 fragment header at offset units of 8 bytes.
func fragmentHeader(next byte, offsetUnits uint16) []byte {
	h := make([]byte, ipv6FragmentLen)
	h[0] = next
	binary.BigEndian.PutUint16(h[2:4], offsetUnits<<3)
	return h
}

// tcpHeader is the part of a TCP header the tests vary.
type tcpHeader struct {
	srcPort, dstPort uint16
	seq, ack         uint32
	flags            byte
	window           uint16
	options          []byte
}

func tcpSegment(h tcpHeader, payload []byte) []byte {
	hl := tcpHeaderLen + len(h.options)
	seg := make([]byte, hl, hl+len(payload))
	binary.BigEndian.PutUint16(seg[0:2], h.srcPort)
	binary.BigEndian.PutUint16(seg[2:4], h.dstPort)
	binary.BigEndian.PutUint32(seg[4:8], h.seq)
	binary.BigEndian.PutUint32(seg[8:12], h.ack)
	seg[12] = byte(hl/ihlUnit) << nibbleBits
	seg[13] = h.flags
	binary.BigEndian.PutUint16(seg[14:16], h.window)
	copy(seg[tcpHeaderLen:], h.options)
	return append(seg, payload...)
}

func udpSegment(srcPort, dstPort uint16, payload []byte) []byte {
	seg := make([]byte, udpHeaderLen, udpHeaderLen+len(payload))
	binary.BigEndian.PutUint16(seg[0:2], srcPort)
	binary.BigEndian.PutUint16(seg[2:4], dstPort)
	binary.BigEndian.PutUint16(seg[4:6], uint16(udpHeaderLen+len(payload)))
	return append(seg, payload...)
}

func icmpMessage(typ, code byte, rest []byte) []byte {
	msg := make([]byte, icmpHeaderLen, icmpHeaderLen+len(rest))
	msg[0], msg[1] = typ, code
	return append(msg, rest...)
}

func echoBody(id, seq uint16) []byte {
	return append(u16(id), u16(seq)...)
}

// v4Frame wraps one IPv4 packet between the test client and server in an
// Ethernet frame.
func v4Frame(proto byte, payload []byte) []byte {
	packet := ipv4Packet(ipv4Header{src: testClientIP, dst: testServerIP, proto: proto, ttl: 64}, payload)
	return ethFrame(testRouterMAC, testClientMAC, etherTypeIPv4, packet)
}

// v6Frame wraps one IPv6 packet between the test client and server in an
// Ethernet frame.
func v6Frame(next byte, payload []byte) []byte {
	packet := ipv6Packet(testClientV6, testServerV6, next, payload)
	return ethFrame(testRouterMAC, testClientMAC, etherTypeIPv6, packet)
}

// arpPacket is an ARP message with Ethernet addresses and IPv4 protocol
// addresses, the usual case.
func arpPacket(op uint16, senderMAC, senderIP, targetMAC, targetIP string) []byte {
	b := make([]byte, arpFixedLen+2*(6+4))
	binary.BigEndian.PutUint16(b[0:2], hwTypeEthernet)
	binary.BigEndian.PutUint16(b[2:4], etherTypeIPv4)
	b[4], b[5] = 6, 4
	binary.BigEndian.PutUint16(b[6:8], op)
	copy(b[8:14], mustMAC(senderMAC))
	copy(b[14:18], mustIP(senderIP))
	copy(b[18:24], mustMAC(targetMAC))
	copy(b[24:28], mustIP(targetIP))
	return b
}

// dnsQuestion and dnsAnswer are the sections of a DNS message the tests
// assemble.
type dnsQuestion struct {
	name  string
	typ   uint16
	class uint16
}

type dnsAnswer struct {
	name  string
	typ   uint16
	class uint16
	ttl   uint32
	data  []byte
}

func dnsName(name string) []byte {
	var b []byte
	for _, label := range strings.Split(strings.TrimSuffix(name, "."), ".") {
		if label == "" {
			continue
		}
		b = append(b, byte(len(label)))
		b = append(b, label...)
	}
	return append(b, 0)
}

func dnsMessage(id, flags uint16, questions []dnsQuestion, answers []dnsAnswer) []byte {
	msg := make([]byte, dnsHeaderLen)
	binary.BigEndian.PutUint16(msg[0:2], id)
	binary.BigEndian.PutUint16(msg[2:4], flags)
	binary.BigEndian.PutUint16(msg[4:6], uint16(len(questions)))
	binary.BigEndian.PutUint16(msg[6:8], uint16(len(answers)))
	for _, q := range questions {
		msg = append(msg, dnsName(q.name)...)
		msg = append(msg, u16(q.typ)...)
		msg = append(msg, u16(q.class)...)
	}
	for _, a := range answers {
		msg = append(msg, dnsName(a.name)...)
		msg = append(msg, u16(a.typ)...)
		msg = append(msg, u16(a.class)...)
		msg = append(msg, u32(a.ttl)...)
		msg = append(msg, u16(uint16(len(a.data)))...)
		msg = append(msg, a.data...)
	}
	return msg
}

// tcpDNS puts a DNS message behind the 2-byte length DNS over TCP uses.
func tcpDNS(msg []byte, prefix uint16) []byte {
	return append(u16(prefix), msg...)
}

// tlsHello assembles a TLS record holding a ClientHello with the given
// server name and ALPN protocols; an empty name or list leaves the
// extension out.
func tlsHello(recordVersion uint16, sni string, alpn []string) []byte {
	var extensions []byte
	if sni != "" {
		name := []byte(sni)
		data := append(u16(uint16(3+len(name))), 0)
		data = append(data, u16(uint16(len(name)))...)
		data = append(data, name...)
		extensions = append(extensions, u16(tlsExtServerName)...)
		extensions = append(extensions, u16(uint16(len(data)))...)
		extensions = append(extensions, data...)
	}
	if len(alpn) > 0 {
		var list []byte
		for _, name := range alpn {
			list = append(list, byte(len(name)))
			list = append(list, name...)
		}
		data := append(u16(uint16(len(list))), list...)
		extensions = append(extensions, u16(tlsExtALPN)...)
		extensions = append(extensions, u16(uint16(len(data)))...)
		extensions = append(extensions, data...)
	}
	body := u16(0x0303)
	body = append(body, bytes.Repeat([]byte{0x42}, tlsRandomLen)...)
	body = append(body, 0) // no session id
	body = append(body, u16(2)...)
	body = append(body, 0x13, 0x01)
	body = append(body, 1, 0) // null compression
	body = append(body, u16(uint16(len(extensions)))...)
	body = append(body, extensions...)
	handshake := append([]byte{tlsClientHello}, u24(len(body))...)
	handshake = append(handshake, body...)
	record := append([]byte{tlsHandshake}, u16(recordVersion)...)
	record = append(record, u16(uint16(len(handshake)))...)
	return append(record, handshake...)
}

// field is the value of one layer field, and fails the test without it.
func field(t *testing.T, f tool.Flow, layer, name string) string {
	t.Helper()
	l := findLayer(t, f, layer)
	for _, h := range l.Fields {
		if h.Name == name {
			return h.Value
		}
	}
	t.Fatalf("layer %q has no field %q, has %v", layer, name, l.Fields)
	return ""
}

// findLayer is one layer of a decoded packet, and fails the test without
// it.
func findLayer(t *testing.T, f tool.Flow, name string) tool.Layer {
	t.Helper()
	if f.Packet == nil {
		t.Fatalf("flow %q has no packet detail", f.Info)
	}
	for _, l := range f.Packet.Layers {
		if l.Name == name {
			return l
		}
	}
	t.Fatalf("flow %q has no layer %q, has %v", f.Info, name, layerNames(f))
	return tool.Layer{}
}

// hasLayer reports whether a decoded packet holds a layer with that name.
func hasLayer(f tool.Flow, name string) bool {
	if f.Packet == nil {
		return false
	}
	for _, l := range f.Packet.Layers {
		if l.Name == name {
			return true
		}
	}
	return false
}

func layerNames(f tool.Flow) []string {
	if f.Packet == nil {
		return nil
	}
	names := make([]string, 0, len(f.Packet.Layers))
	for _, l := range f.Packet.Layers {
		names = append(names, l.Name)
	}
	return names
}
