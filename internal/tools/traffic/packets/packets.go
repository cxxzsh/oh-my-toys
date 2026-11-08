// Package packets turns captured packets into flows of the traffic tool:
// one flow per packet, with the protocol layers the packet's bytes hold,
// the raw bytes, and a one-line summary in the style of Wireshark. Decoding
// never fails: a packet cut short or contradicting itself is recorded as
// far as its bytes allow, and Info says where reading stopped.
package packets

import (
	"fmt"
	"net"
	"strconv"
	"unicode/utf8"

	"github.com/cxxzsh/oh-my-toys/internal/platform/pcap"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// Recorder is where decoded packets are written; flowlog.Log implements it.
type Recorder interface{ Put(tool.Flow) uint64 }

// Proto is the highest layer Decode understood from a packet.
const (
	protoUnknown  = "unknown"
	protoEthernet = "Ethernet"
	protoIPv4     = "IPv4"
	protoIPv6     = "IPv6"
	protoARP      = "ARP"
	protoTCP      = "TCP"
	protoUDP      = "UDP"
	protoICMP     = "ICMP"
	protoICMPv6   = "ICMPv6"
	protoDNS      = "DNS"
	protoTLS      = "TLS"
	protoHTTP     = "HTTP"
)

// Field labels shared by several layers. They are protocol vocabulary, the
// words Wireshark shows, not translatable UI strings.
const (
	fieldSource      = "Source"
	fieldDestination = "Destination"
	fieldType        = "Type"
	fieldVersion     = "Version"
	fieldChecksum    = "Checksum"
	fieldHeaderLen   = "Header length"
	fieldLength      = "Length"
	fieldSrcPort     = "Source port"
	fieldDstPort     = "Destination port"
)

// Decode turns one packet into a flow. The row names the highest protocol
// that was understood and says what the packet is; Packet holds one layer
// per protocol that was decoded and a copy of the bytes.
func Decode(p pcap.Packet) tool.Flow {
	var d decoder
	d.link(p)
	if d.proto == "" {
		d.proto = protoUnknown
	}
	if d.info == "" {
		d.info = d.proto
	}
	return tool.Flow{
		FlowRow: tool.FlowRow{
			Source:  tool.SourcePacket,
			Started: p.Time,
			Method:  d.method,
			Host:    d.host,
			Path:    d.path,
			Proto:   d.proto,
			Status:  d.status,
			Size:    int64(p.Length),
			State:   tool.FlowDone,
			Info:    d.info,
		},
		Packet: &tool.PacketDetail{Layers: d.layers, Raw: rawBytes(p.Data)},
	}
}

// decoder collects what one packet turned out to be while its bytes are
// read layer by layer.
type decoder struct {
	layers []tool.Layer
	proto  string
	host   string
	info   string
	method string
	path   string
	status int
}

// link hands the bytes to the parser the capture's link type names.
func (d *decoder) link(p pcap.Packet) {
	switch p.Link {
	case pcap.LinkEthernet:
		d.ethernet(p.Data)
	case pcap.LinkNull:
		d.null(p.Data)
	case pcap.LinkRaw:
		d.network(p.Data)
	default:
		d.info = "Unknown link type " + strconv.Itoa(int(p.Link))
	}
}

// layer appends one protocol layer with its fields, in order.
func (d *decoder) layer(name string, f *fieldSet) {
	var fields []tool.Header
	if f != nil {
		fields = f.list
	}
	d.layers = append(d.layers, tool.Layer{Name: name, Fields: fields})
}

// truncated says the packet ends before what it announces.
func (d *decoder) truncated(what string) {
	d.info = "[truncated] " + what
}

// malformed says the packet's bytes contradict themselves.
func (d *decoder) malformed(what string) {
	d.info = "Malformed " + what
}

// fieldSet builds the name/value pairs of one layer in the order they are
// shown.
type fieldSet struct{ list []tool.Header }

func (f *fieldSet) add(name, value string) {
	f.list = append(f.list, tool.Header{Name: name, Value: value})
}

func (f *fieldSet) addInt(name string, value int) {
	f.add(name, strconv.Itoa(value))
}

// rawBytes copies at most MaxBody of the capture, so a flow never keeps the
// capture buffer alive.
func rawBytes(data []byte) []byte {
	if len(data) > tool.MaxBody {
		data = data[:tool.MaxBody]
	}
	raw := make([]byte, len(data))
	copy(raw, data)
	return raw
}

// ipString shows an address the way the list shows it.
func ipString(b []byte) string { return net.IP(b).String() }

// hostPort is an address with its port, bracketed for IPv6.
func hostPort(ip []byte, port uint16) string {
	return net.JoinHostPort(ipString(ip), strconv.FormatUint(uint64(port), 10))
}

// isText reports whether b is printable UTF-8, so a field never holds
// captured binary data.
func isText(b []byte) bool {
	if !utf8.Valid(b) {
		return false
	}
	for _, c := range b {
		if c < ' ' && c != '\t' {
			return false
		}
	}
	return true
}

// hex16 shows a 16-bit field the way packet diagrams do.
func hex16(b []byte) string {
	if len(b) < 2 {
		return ""
	}
	return fmt.Sprintf("0x%02x%02x", b[0], b[1])
}
