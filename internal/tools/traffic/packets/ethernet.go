package packets

import (
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
)

// The link layer: Ethernet with its 802.1Q tags, ARP on top of it, and the
// 4-byte address family header of the BSD loopback.

const (
	layerEthernet   = "Ethernet II"
	layerVLAN       = "802.1Q Virtual LAN"
	layerARP        = "Address Resolution Protocol"
	layerLoopback   = "Null/Loopback"
	etherHeaderLen  = 14
	vlanTagLen      = 4
	arpFixedLen     = 8
	nullHeaderLen   = 4
	etherTypeIPv4   = 0x0800
	etherTypeARP    = 0x0806
	etherTypeIPv6   = 0x86dd
	etherTypeVLAN   = 0x8100
	etherTypeQinQ   = 0x88a8
	arpOpRequest    = 1
	arpOpReply      = 2
	hwTypeEthernet  = 1
	afInet          = 2  // AF_INET, in the loopback header
	afInet6Darwin   = 30 // AF_INET6 on macOS
	afInet6FreeBSD  = 28 // AF_INET6 on FreeBSD
	afInet6NetBSD   = 24 // AF_INET6 on NetBSD and OpenBSD
	vlanPriorityBit = 13
	vlanDEIBit      = 12
	vlanIDMask      = 0x0fff
)

// ethernet decodes a frame and hands its payload to the EtherType it names.
func (d *decoder) ethernet(data []byte) {
	if len(data) < etherHeaderLen {
		d.truncated("Ethernet header")
		return
	}
	f := &fieldSet{}
	f.add(fieldDestination, net.HardwareAddr(data[0:6]).String())
	f.add(fieldSource, net.HardwareAddr(data[6:12]).String())
	typ := binary.BigEndian.Uint16(data[12:14])
	f.add(fieldType, etherTypeText(typ))
	d.layer(layerEthernet, f)
	d.proto = protoEthernet
	payload, typ, ok := d.vlan(data[etherHeaderLen:], typ)
	if !ok {
		return
	}
	switch typ {
	case etherTypeIPv4:
		d.ipv4(payload)
	case etherTypeIPv6:
		d.ipv6(payload)
	case etherTypeARP:
		d.arp(payload)
	default:
		d.info = "Ethernet type " + etherTypeText(typ)
	}
}

// vlan skips the 802.1Q tags between the Ethernet header and the EtherType
// they carry, and returns the innermost type and payload.
func (d *decoder) vlan(data []byte, typ uint16) ([]byte, uint16, bool) {
	for typ == etherTypeVLAN || typ == etherTypeQinQ {
		if len(data) < vlanTagLen {
			d.truncated("802.1Q tag")
			return nil, 0, false
		}
		tci := binary.BigEndian.Uint16(data[0:2])
		f := &fieldSet{}
		f.addInt("Priority", int(tci)>>vlanPriorityBit)
		f.add("Drop eligible", boolText(int(tci)&(1<<vlanDEIBit) != 0))
		f.addInt("ID", int(tci&vlanIDMask))
		d.layer(layerVLAN, f)
		typ = binary.BigEndian.Uint16(data[2:4])
		data = data[vlanTagLen:]
	}
	return data, typ, true
}

// arp decodes an ARP request or reply, which is all this package sees of
// it: the addresses and what the message asks for.
func (d *decoder) arp(data []byte) {
	if len(data) < arpFixedLen {
		d.truncated("ARP header")
		return
	}
	op := binary.BigEndian.Uint16(data[6:8])
	hwLen, protoLen := int(data[4]), int(data[5])
	f := &fieldSet{}
	f.add("Hardware type", hwTypeText(binary.BigEndian.Uint16(data[0:2])))
	f.add("Protocol type", etherTypeText(binary.BigEndian.Uint16(data[2:4])))
	f.addInt("Hardware length", hwLen)
	f.addInt("Protocol length", protoLen)
	f.add("Operation", arpOpText(op))
	d.proto = protoARP
	addrs := data[arpFixedLen:]
	if len(addrs) < 2*(hwLen+protoLen) {
		d.layer(layerARP, f)
		d.truncated("ARP addresses")
		return
	}
	senderHW := net.HardwareAddr(addrs[:hwLen]).String()
	senderIP := ipString(addrs[hwLen : hwLen+protoLen])
	targetHW := net.HardwareAddr(addrs[hwLen+protoLen : 2*hwLen+protoLen]).String()
	targetIP := ipString(addrs[2*hwLen+protoLen : 2*(hwLen+protoLen)])
	f.add("Sender MAC", senderHW)
	f.add("Sender IP", senderIP)
	f.add("Target MAC", targetHW)
	f.add("Target IP", targetIP)
	d.layer(layerARP, f)
	d.host = targetIP
	d.info = arpSummary(op, senderIP, senderHW, targetIP)
}

// null decodes the BSD loopback header: a 4-byte address family in the
// host's byte order, which libpcap also accepts byte-swapped.
func (d *decoder) null(data []byte) {
	if len(data) < nullHeaderLen {
		d.truncated("loopback header")
		return
	}
	fam := binary.LittleEndian.Uint32(data[:nullHeaderLen])
	if swapped := binary.BigEndian.Uint32(data[:nullHeaderLen]); !knownFamily(fam) && knownFamily(swapped) {
		fam = swapped
	}
	f := &fieldSet{}
	f.add("Family", familyText(fam))
	d.layer(layerLoopback, f)
	switch fam {
	case afInet, afInet6Darwin, afInet6FreeBSD, afInet6NetBSD:
		d.network(data[nullHeaderLen:])
	default:
		d.info = "Loopback address family " + strconv.FormatUint(uint64(fam), 10)
	}
}

// knownFamily reports whether fam is one of the families a loopback header
// can carry, decided before the byte order is fixed.
func knownFamily(fam uint32) bool {
	switch fam {
	case afInet, afInet6Darwin, afInet6FreeBSD, afInet6NetBSD:
		return true
	default:
		return false
	}
}

// etherTypeText names the EtherTypes this package walks through and shows
// the rest as the number they are.
func etherTypeText(t uint16) string {
	switch t {
	case etherTypeIPv4:
		return "IPv4 (0x0800)"
	case etherTypeARP:
		return "ARP (0x0806)"
	case etherTypeIPv6:
		return "IPv6 (0x86dd)"
	case etherTypeVLAN:
		return "802.1Q (0x8100)"
	case etherTypeQinQ:
		return "802.1ad (0x88a8)"
	default:
		return fmt.Sprintf("0x%04x", t)
	}
}

func familyText(fam uint32) string {
	switch fam {
	case afInet:
		return "IPv4 (" + strconv.FormatUint(uint64(fam), 10) + ")"
	case afInet6Darwin, afInet6FreeBSD, afInet6NetBSD:
		return "IPv6 (" + strconv.FormatUint(uint64(fam), 10) + ")"
	default:
		return strconv.FormatUint(uint64(fam), 10)
	}
}

func hwTypeText(t uint16) string {
	if t == hwTypeEthernet {
		return "Ethernet (" + strconv.FormatUint(uint64(t), 10) + ")"
	}
	return strconv.FormatUint(uint64(t), 10)
}

func arpOpText(op uint16) string {
	switch op {
	case arpOpRequest:
		return "Request (1)"
	case arpOpReply:
		return "Reply (2)"
	default:
		return strconv.FormatUint(uint64(op), 10)
	}
}

// arpSummary reads as the sentence Wireshark shows: who is asked for an
// address, or which address a reply claims.
func arpSummary(op uint16, senderIP, senderHW, targetIP string) string {
	switch op {
	case arpOpRequest:
		return "Who has " + targetIP + "? Tell " + senderIP
	case arpOpReply:
		return senderIP + " is at " + senderHW
	default:
		return arpOpText(op)
	}
}

func boolText(v bool) string {
	if v {
		return "Yes"
	}
	return "No"
}
