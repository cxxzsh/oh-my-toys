package packets

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

// IPv4 and IPv6: their fixed headers, the rule that only the first IPv4
// fragment carries a transport header, and the IPv6 extension headers that
// sit between the IPv6 header and it.

const (
	layerIPv4       = "Internet Protocol Version 4"
	layerIPv6       = "Internet Protocol Version 6"
	ipVersion4      = 4
	ipVersion6      = 6
	ipv4HeaderLen   = 20
	ipv6HeaderLen   = 40
	nibbleBits      = 4
	headerLenMask   = 0x0f
	ihlUnit         = 4
	fragOffsetMask  = 0x1fff
	ipv4FlagRes     = 0x8000
	ipv4FlagDF      = 0x4000
	ipv4FlagMF      = 0x2000
	ipv6FragmentLen = 8
	fragmentUnit    = 8
	extHeaderUnit   = 8
	// maxExtensionHeaders bounds the walk through IPv6 extension headers:
	// a real packet never chains more, and the bound keeps a malformed
	// loop cheap.
	maxExtensionHeaders = 8
)

// IP protocol numbers.
const (
	numICMP     = 1
	numTCP      = 6
	numUDP      = 17
	numICMPv6   = 58
	numNoNext   = 59
	protoAH     = 51
	protoESP    = 50
	extHopByHop = 0
	extRouting  = 43
	extFragment = 44
	extDestOpts = 60
)

// network decodes an IP header, whose version nibble says which it is.
func (d *decoder) network(data []byte) {
	if len(data) == 0 {
		d.truncated("IP header")
		return
	}
	switch data[0] >> nibbleBits {
	case ipVersion4:
		d.ipv4(data)
	case ipVersion6:
		d.ipv6(data)
	default:
		d.info = "IP version " + strconv.Itoa(int(data[0]>>nibbleBits))
	}
}

func (d *decoder) ipv4(data []byte) {
	if len(data) < ipv4HeaderLen {
		d.truncated("IPv4 header")
		return
	}
	hl := int(data[0]&headerLenMask) * ihlUnit
	if data[0]>>nibbleBits != ipVersion4 || hl < ipv4HeaderLen {
		d.malformed("IPv4 header")
		return
	}
	totalLen := int(binary.BigEndian.Uint16(data[2:4]))
	flagsFrag := binary.BigEndian.Uint16(data[6:8])
	proto := data[9]
	src, dst := data[12:16], data[16:20]
	f := &fieldSet{}
	f.addInt(fieldVersion, ipVersion4)
	f.addInt(fieldHeaderLen, hl)
	f.addInt("Total length", totalLen)
	f.add("Flags", ipv4Flags(flagsFrag))
	f.addInt("Fragment offset", int(flagsFrag&fragOffsetMask)*fragmentUnit)
	f.addInt("TTL", int(data[8]))
	f.add("Protocol", protocolText(proto))
	f.add(fieldSource, ipString(src))
	f.add(fieldDestination, ipString(dst))
	d.layer(layerIPv4, f)
	d.proto = protoIPv4
	d.host = ipString(dst)
	if hl > len(data) {
		d.truncated("IPv4 header")
		return
	}
	if offset := int(flagsFrag&fragOffsetMask) * fragmentUnit; offset != 0 {
		d.info = "IPv4 fragment, offset " + strconv.Itoa(offset)
		return
	}
	d.transport(proto, payloadOf(data, hl, totalLen), dst)
}

func (d *decoder) ipv6(data []byte) {
	if len(data) < ipv6HeaderLen {
		d.truncated("IPv6 header")
		return
	}
	if data[0]>>nibbleBits != ipVersion6 {
		d.malformed("IPv6 header")
		return
	}
	head := binary.BigEndian.Uint32(data[0:4])
	next := data[6]
	totalLen := ipv6HeaderLen + int(binary.BigEndian.Uint16(data[4:6]))
	src, dst := data[8:24], data[24:40]
	f := &fieldSet{}
	f.addInt(fieldVersion, ipVersion6)
	f.addInt("Traffic class", int(head>>20)&0xff)
	f.add("Flow label", fmt.Sprintf("0x%05x", head&0xfffff))
	f.addInt("Payload length", totalLen-ipv6HeaderLen)
	f.add("Next header", protocolText(next))
	f.addInt("Hop limit", int(data[7]))
	f.add(fieldSource, ipString(src))
	f.add(fieldDestination, ipString(dst))
	d.layer(layerIPv6, f)
	d.proto = protoIPv6
	d.host = ipString(dst)
	next, payload, ok := d.extensions(next, payloadOf(data, ipv6HeaderLen, totalLen))
	if !ok {
		return
	}
	d.transport(next, payload, dst)
}

// extensions walks the IPv6 extension headers up to the transport header.
// It stops at a fragment beyond the first, which carries no transport
// header here, and wherever the bytes end.
func (d *decoder) extensions(next byte, payload []byte) (byte, []byte, bool) {
	for range maxExtensionHeaders {
		switch next {
		case extHopByHop, extRouting, extDestOpts:
			if len(payload) < 2 {
				d.truncated("IPv6 extension header")
				return 0, nil, false
			}
			size := (int(payload[1]) + 1) * extHeaderUnit
			if size > len(payload) {
				d.truncated("IPv6 extension header")
				return 0, nil, false
			}
			next, payload = payload[0], payload[size:]
		case extFragment:
			if len(payload) < ipv6FragmentLen {
				d.truncated("IPv6 fragment header")
				return 0, nil, false
			}
			if offset := int(binary.BigEndian.Uint16(payload[2:4]) >> 3); offset != 0 {
				d.info = "IPv6 fragment, offset " + strconv.Itoa(offset*fragmentUnit)
				return 0, nil, false
			}
			next, payload = payload[0], payload[ipv6FragmentLen:]
		default:
			return next, payload, true
		}
	}
	d.malformed("IPv6 extension headers")
	return 0, nil, false
}

// transport hands the IP payload to the transport protocol it names.
func (d *decoder) transport(proto byte, payload, dst []byte) {
	switch proto {
	case numTCP:
		d.tcp(payload, dst)
	case numUDP:
		d.udp(payload, dst)
	case numICMP:
		d.icmp(payload)
	case numICMPv6:
		d.icmpV6(payload)
	case numNoNext:
		d.info = "No next header"
	case protoAH, protoESP:
		d.info = "IPsec, not decoded"
	default:
		d.info = "IP protocol " + strconv.Itoa(int(proto))
	}
}

// payloadOf cuts the payload the header announced, and keeps what the
// capture holds when that is less (a snapped packet) or contradicts it.
func payloadOf(data []byte, hl, totalLen int) []byte {
	payload := data[hl:]
	if totalLen >= hl && totalLen < len(data) {
		payload = data[hl:totalLen]
	}
	return payload
}

// protocolText names the IP protocol number as Wireshark does.
func protocolText(num byte) string {
	switch num {
	case numICMP:
		return "ICMP (1)"
	case numTCP:
		return "TCP (6)"
	case numUDP:
		return "UDP (17)"
	case numICMPv6:
		return "ICMPv6 (58)"
	case numNoNext:
		return "No next header (59)"
	default:
		return strconv.Itoa(int(num))
	}
}

// ipv4Flags spells out the flags of the fragment field.
func ipv4Flags(v uint16) string {
	parts := make([]string, 0, 3)
	if v&ipv4FlagRes != 0 {
		parts = append(parts, "Reserved")
	}
	if v&ipv4FlagDF != 0 {
		parts = append(parts, "DF")
	}
	if v&ipv4FlagMF != 0 {
		parts = append(parts, "MF")
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}
