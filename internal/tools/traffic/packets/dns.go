package packets

import (
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"strings"

	"golang.org/x/net/dns/dnsmessage"
)

// DNS over UDP and mDNS, and the same message behind the 2-byte length
// prefix of DNS over TCP. Only what parses is shown; sniffer captures cut
// messages often.

const (
	layerDNS        = "Domain Name System"
	dnsPort         = 53
	mdnsPort        = 5353
	dnsHeaderLen    = 12
	dnsTCPPrefixLen = 2
)

// isDNSAddr reports whether a port pair carries DNS or mDNS.
func isDNSAddr(a, b uint16) bool {
	return a == dnsPort || b == dnsPort || a == mdnsPort || b == mdnsPort
}

// dnsStream decodes DNS over TCP, where a 2-byte length comes first. It
// reports false when the whole message is not in this segment.
func (d *decoder) dnsStream(payload []byte) bool {
	if len(payload) < dnsTCPPrefixLen {
		return false
	}
	size := int(binary.BigEndian.Uint16(payload[:dnsTCPPrefixLen]))
	if size < dnsHeaderLen || dnsTCPPrefixLen+size > len(payload) {
		return false
	}
	d.dns(payload[dnsTCPPrefixLen : dnsTCPPrefixLen+size])
	return true
}

// dns adds a field per question and answer, and says in Info what the
// message asks or answers.
func (d *decoder) dns(msg []byte) {
	var p dnsmessage.Parser
	header, err := p.Start(msg)
	if err != nil {
		d.dnsFailure(msg)
		return
	}
	f := dnsHeaderFields(header)
	questions, ok := dnsSections(&p, f)
	d.layer(layerDNS, f)
	d.proto = protoDNS
	if !ok {
		d.malformed("DNS message")
		return
	}
	d.info = dnsSummary(header, questions)
}

// dnsHeaderFields is the header of a message, plus the flag bits in words.
func dnsHeaderFields(header dnsmessage.Header) *fieldSet {
	f := &fieldSet{}
	f.add("Transaction ID", fmt.Sprintf("0x%04x", header.ID))
	f.add("Flags", dnsFlags(header))
	return f
}

// dnsSections adds the question and answer sections, and reports whether
// the whole message parsed.
func dnsSections(p *dnsmessage.Parser, f *fieldSet) ([]dnsmessage.Question, bool) {
	questions, err := p.AllQuestions()
	if err != nil {
		return nil, false
	}
	for _, q := range questions {
		f.add("Question", dnsTypeText(q.Type)+" "+dnsNameText(q.Name)+" "+dnsClassText(q.Class))
	}
	answers, err := p.AllAnswers()
	if err != nil {
		return questions, false
	}
	for _, a := range answers {
		f.add("Answer", dnsTypeText(a.Header.Type)+" "+dnsNameText(a.Header.Name)+" "+answerData(a.Body))
	}
	return questions, true
}

// dnsSummary reads as Wireshark's Info: the operation, then each question
// as type and name.
func dnsSummary(header dnsmessage.Header, questions []dnsmessage.Question) string {
	var b strings.Builder
	b.WriteString(opCodeText(header.OpCode))
	if header.Response {
		b.WriteString(" response")
	}
	for _, q := range questions {
		b.WriteString(" " + dnsTypeText(q.Type) + " " + dnsNameText(q.Name))
	}
	if header.RCode != dnsmessage.RCodeSuccess {
		b.WriteString(", " + rCodeText(header.RCode))
	}
	return b.String()
}

// dnsFlags says what the header's bits mean.
func dnsFlags(header dnsmessage.Header) string {
	parts := []string{opCodeText(header.OpCode)}
	if header.Response {
		parts = append(parts, "response")
	}
	if header.Authoritative {
		parts = append(parts, "authoritative")
	}
	if header.Truncated {
		parts = append(parts, "truncated")
	}
	if header.RecursionDesired {
		parts = append(parts, "recursion desired")
	}
	if header.RecursionAvailable {
		parts = append(parts, "recursion available")
	}
	if header.RCode != dnsmessage.RCodeSuccess {
		parts = append(parts, rCodeText(header.RCode))
	}
	return strings.Join(parts, ", ")
}

// dnsFailure reports a message that does not parse, telling one cut short
// from one that contradicts itself.
func (d *decoder) dnsFailure(msg []byte) {
	d.proto = protoDNS
	if len(msg) < dnsHeaderLen {
		d.truncated("DNS message")
		return
	}
	d.malformed("DNS message")
}

// answerData shows the data of one answer as its own syntax: an address, a
// name, or the fields of the more involved types.
func answerData(body dnsmessage.ResourceBody) string {
	switch b := body.(type) {
	case *dnsmessage.AResource:
		return net.IP(b.A[:]).String()
	case *dnsmessage.AAAAResource:
		return net.IP(b.AAAA[:]).String()
	case *dnsmessage.CNAMEResource:
		return dnsNameText(b.CNAME)
	case *dnsmessage.NSResource:
		return dnsNameText(b.NS)
	case *dnsmessage.PTRResource:
		return dnsNameText(b.PTR)
	case *dnsmessage.MXResource:
		return strconv.Itoa(int(b.Pref)) + " " + dnsNameText(b.MX)
	case *dnsmessage.TXTResource:
		return strings.Join(b.TXT, " ")
	case *dnsmessage.SRVResource:
		return fmt.Sprintf("%d %d %d %s", b.Priority, b.Weight, b.Port, dnsNameText(b.Target))
	case *dnsmessage.SOAResource:
		return dnsNameText(b.NS) + " " + dnsNameText(b.MBox)
	case *dnsmessage.OPTResource:
		return strconv.Itoa(len(b.Options)) + " options"
	case *dnsmessage.UnknownResource:
		return strconv.Itoa(len(b.Data)) + " bytes"
	default:
		return ""
	}
}

// opCodeText names the operation of a message.
func opCodeText(op dnsmessage.OpCode) string {
	switch op {
	case 0:
		return "Standard query"
	case 1:
		return "Inverse query"
	case 2:
		return "Status"
	case 4:
		return "Notify"
	case 5:
		return "Update"
	default:
		return "Opcode " + strconv.Itoa(int(op))
	}
}

// rCodeText names the error of a response.
func rCodeText(rcode dnsmessage.RCode) string {
	switch rcode {
	case dnsmessage.RCodeFormatError:
		return "Format error"
	case dnsmessage.RCodeServerFailure:
		return "Server failure"
	case dnsmessage.RCodeNameError:
		return "Name error"
	case dnsmessage.RCodeNotImplemented:
		return "Not implemented"
	case dnsmessage.RCodeRefused:
		return "Refused"
	default:
		return "RCode " + strconv.Itoa(int(rcode))
	}
}

// dnsTypeText names a record type the way dig shows it.
func dnsTypeText(t dnsmessage.Type) string {
	switch t {
	case dnsmessage.TypeA:
		return "A"
	case dnsmessage.TypeNS:
		return "NS"
	case dnsmessage.TypeCNAME:
		return "CNAME"
	case dnsmessage.TypeSOA:
		return "SOA"
	case dnsmessage.TypePTR:
		return "PTR"
	case dnsmessage.TypeMX:
		return "MX"
	case dnsmessage.TypeTXT:
		return "TXT"
	case dnsmessage.TypeAAAA:
		return "AAAA"
	case dnsmessage.TypeSRV:
		return "SRV"
	case dnsmessage.TypeOPT:
		return "OPT"
	case dnsmessage.TypeSVCB:
		return "SVCB"
	case dnsmessage.TypeHTTPS:
		return "HTTPS"
	case dnsmessage.TypeALL:
		return "ANY"
	default:
		return "Type " + strconv.Itoa(int(t))
	}
}

// dnsNameText shows a name the way dig does, without the root dot unless
// the name is the root itself.
func dnsNameText(name dnsmessage.Name) string {
	text := name.String()
	if text == "." || !strings.HasSuffix(text, ".") {
		return text
	}
	return strings.TrimSuffix(text, ".")
}

// dnsClassText names the class of a question or record.
func dnsClassText(c dnsmessage.Class) string {
	switch c {
	case dnsmessage.ClassINET:
		return "IN"
	case dnsmessage.ClassCSNET:
		return "CS"
	case dnsmessage.ClassCHAOS:
		return "CH"
	case dnsmessage.ClassHESIOD:
		return "HS"
	case dnsmessage.ClassANY:
		return "ANY"
	default:
		return "Class " + strconv.Itoa(int(c))
	}
}
