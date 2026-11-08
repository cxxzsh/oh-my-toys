package packets

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

// The transport layer: TCP, UDP, ICMP and ICMPv6, and the place where the
// protocols above them are looked for.

const (
	layerTCP   = "Transmission Control Protocol"
	layerUDP   = "User Datagram Protocol"
	layerICMP  = "Internet Control Message Protocol"
	layerICMP6 = "Internet Control Message Protocol v6"

	tcpHeaderLen  = 20
	udpHeaderLen  = 8
	icmpHeaderLen = 4
	icmpEchoLen   = 8
)

// TCP flags, which tcpFlagsText lists in this order.
const (
	tcpFlagFIN byte = 1 << iota
	tcpFlagSYN
	tcpFlagRST
	tcpFlagPSH
	tcpFlagACK
	tcpFlagURG
	tcpFlagECE
	tcpFlagCWR
)

// tcpFlagCount is the number of flag bits tcpFlagsText lists.
const tcpFlagCount = 8

// ICMP and ICMPv6 message types this package names.
const (
	icmpEchoReply       = 0
	icmpDestUnreachable = 3
	icmpEchoRequest     = 8
	icmpTimeExceeded    = 11

	icmpv6EchoRequest           = 128
	icmpv6EchoReply             = 129
	icmpv6NeighborSolicitation  = 135
	icmpv6NeighborAdvertisement = 136
	icmpv6TargetOffset          = 8
	icmpv6TargetAddressLen      = 16
)

// tcp decodes a TCP header and looks above it for DNS, TLS and HTTP.
func (d *decoder) tcp(data, dst []byte) {
	if len(data) < tcpHeaderLen {
		d.truncated("TCP header")
		return
	}
	srcPort := binary.BigEndian.Uint16(data[0:2])
	dstPort := binary.BigEndian.Uint16(data[2:4])
	hl := int(data[12]>>nibbleBits) * ihlUnit
	if hl < tcpHeaderLen {
		d.malformed("TCP header")
		return
	}
	flags := tcpFlagsText(data[13])
	f := &fieldSet{}
	f.addInt(fieldSrcPort, int(srcPort))
	f.addInt(fieldDstPort, int(dstPort))
	f.add("Sequence number", strconv.FormatUint(uint64(binary.BigEndian.Uint32(data[4:8])), 10))
	f.add("Acknowledgment number", strconv.FormatUint(uint64(binary.BigEndian.Uint32(data[8:12])), 10))
	f.addInt(fieldHeaderLen, hl)
	f.add("Flags", flags)
	f.addInt("Window", int(binary.BigEndian.Uint16(data[14:16])))
	d.layer(layerTCP, f)
	d.proto = protoTCP
	d.host = hostPort(dst, dstPort)
	if hl > len(data) {
		d.truncated("TCP header")
		return
	}
	payload := data[hl:]
	d.info = fmt.Sprintf("%d → %d [%s] Len=%d", srcPort, dstPort, flags, len(payload))
	d.aboveTCP(srcPort, dstPort, payload)
}

// aboveTCP looks for the protocols this package knows above TCP. When none
// matches, the TCP summary stays in Info.
func (d *decoder) aboveTCP(srcPort, dstPort uint16, payload []byte) {
	if len(payload) == 0 {
		return
	}
	if srcPort == dnsPort || dstPort == dnsPort {
		if d.dnsStream(payload) {
			return
		}
	}
	if d.tlsRecord(payload) {
		return
	}
	d.http(payload)
}

// udp decodes a UDP header and looks above it for DNS and mDNS.
func (d *decoder) udp(data, dst []byte) {
	if len(data) < udpHeaderLen {
		d.truncated("UDP header")
		return
	}
	srcPort := binary.BigEndian.Uint16(data[0:2])
	dstPort := binary.BigEndian.Uint16(data[2:4])
	f := &fieldSet{}
	f.addInt(fieldSrcPort, int(srcPort))
	f.addInt(fieldDstPort, int(dstPort))
	f.addInt(fieldLength, int(binary.BigEndian.Uint16(data[4:6])))
	f.add(fieldChecksum, hex16(data[6:8]))
	d.layer(layerUDP, f)
	d.proto = protoUDP
	d.host = hostPort(dst, dstPort)
	payload := data[udpHeaderLen:]
	d.info = fmt.Sprintf("%d → %d Len=%d", srcPort, dstPort, len(payload))
	if isDNSAddr(srcPort, dstPort) && len(payload) > 0 {
		d.dns(payload)
	}
}

// icmp decodes an ICMP message: the type and the code say what it is.
func (d *decoder) icmp(data []byte) {
	if len(data) < icmpHeaderLen {
		d.truncated("ICMP header")
		return
	}
	f := &fieldSet{}
	f.add(fieldType, icmpTypeText(data[0]))
	f.add("Code", icmpCodeText(data[0], data[1]))
	f.add(fieldChecksum, hex16(data[2:4]))
	info := echoFields(f, data, data[0] == icmpEchoReply || data[0] == icmpEchoRequest)
	d.layer(layerICMP, f)
	d.proto = protoICMP
	d.info = icmpTypeText(data[0]) + info
}

// icmpV6 decodes an ICMPv6 message, including the target of a neighbor
// discovery message.
func (d *decoder) icmpV6(data []byte) {
	if len(data) < icmpHeaderLen {
		d.truncated("ICMPv6 header")
		return
	}
	typ := data[0]
	f := &fieldSet{}
	f.add(fieldType, icmpV6TypeText(typ))
	f.add("Code", strconv.Itoa(int(data[1])))
	f.add(fieldChecksum, hex16(data[2:4]))
	info := echoFields(f, data, typ == icmpv6EchoRequest || typ == icmpv6EchoReply)
	if target, ok := neighborTarget(data, typ); ok {
		f.add("Target address", target)
		info += " for " + target
	}
	d.layer(layerICMP6, f)
	d.proto = protoICMPv6
	d.info = icmpV6TypeText(typ) + info
}

// echoFields adds the identifier and sequence of an echo message, and
// returns the same values as a suffix for Info. Echo messages of either IP
// version carry them at the same offset.
func echoFields(f *fieldSet, data []byte, echo bool) string {
	if !echo || len(data) < icmpEchoLen {
		return ""
	}
	id := binary.BigEndian.Uint16(data[4:6])
	seq := binary.BigEndian.Uint16(data[6:8])
	f.add("Identifier", fmt.Sprintf("0x%04x", id))
	f.addInt("Sequence number", int(seq))
	return fmt.Sprintf(" id=0x%04x seq=%d", id, seq)
}

// neighborTarget is the address an ICMPv6 neighbor discovery message
// asks about, when it is all there.
func neighborTarget(data []byte, typ byte) (string, bool) {
	if typ != icmpv6NeighborSolicitation && typ != icmpv6NeighborAdvertisement {
		return "", false
	}
	if len(data) < icmpv6TargetOffset+icmpv6TargetAddressLen {
		return "", false
	}
	return ipString(data[icmpv6TargetOffset : icmpv6TargetOffset+icmpv6TargetAddressLen]), true
}

// tcpFlagsText names the flags in the order Wireshark lists them.
func tcpFlagsText(flags byte) string {
	names := make([]string, 0, tcpFlagCount)
	for _, flag := range [...]struct {
		bit  byte
		name string
	}{
		{tcpFlagFIN, "FIN"},
		{tcpFlagSYN, "SYN"},
		{tcpFlagRST, "RST"},
		{tcpFlagPSH, "PSH"},
		{tcpFlagACK, "ACK"},
		{tcpFlagURG, "URG"},
		{tcpFlagECE, "ECE"},
		{tcpFlagCWR, "CWR"},
	} {
		if flags&flag.bit != 0 {
			names = append(names, flag.name)
		}
	}
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}

func icmpTypeText(typ byte) string {
	switch typ {
	case icmpEchoReply:
		return "Echo (ping) reply"
	case icmpDestUnreachable:
		return "Destination unreachable"
	case icmpEchoRequest:
		return "Echo (ping) request"
	case icmpTimeExceeded:
		return "Time-to-live exceeded"
	default:
		return "ICMP type " + strconv.Itoa(int(typ))
	}
}

// icmpCodeText names the codes that change what a message means.
func icmpCodeText(typ, code byte) string {
	switch typ {
	case icmpDestUnreachable:
		if name, ok := unreachableCode(code); ok {
			return name
		}
	case icmpTimeExceeded:
		if name, ok := timeExceededCode(code); ok {
			return name
		}
	}
	return strconv.Itoa(int(code))
}

func unreachableCode(code byte) (string, bool) {
	switch code {
	case 0:
		return "Network unreachable (0)", true
	case 1:
		return "Host unreachable (1)", true
	case 3:
		return "Port unreachable (3)", true
	case 4:
		return "Fragmentation needed (4)", true
	case 13:
		return "Communication administratively filtered (13)", true
	default:
		return "", false
	}
}

func timeExceededCode(code byte) (string, bool) {
	switch code {
	case 0:
		return "Time-to-live exceeded in transit (0)", true
	case 1:
		return "Fragment reassembly time exceeded (1)", true
	default:
		return "", false
	}
}

func icmpV6TypeText(typ byte) string {
	switch typ {
	case 1:
		return "Destination unreachable"
	case 2:
		return "Packet too big"
	case 3:
		return "Time exceeded"
	case 4:
		return "Parameter problem"
	case icmpv6EchoRequest:
		return "Echo (ping) request"
	case icmpv6EchoReply:
		return "Echo (ping) reply"
	case 133:
		return "Router solicitation"
	case 134:
		return "Router advertisement"
	case icmpv6NeighborSolicitation:
		return "Neighbor solicitation"
	case icmpv6NeighborAdvertisement:
		return "Neighbor advertisement"
	case 137:
		return "Redirect"
	default:
		return "ICMPv6 type " + strconv.Itoa(int(typ))
	}
}
