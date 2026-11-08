package packets

import (
	"encoding/hex"
	"testing"
)

// capturedFrame is a real captured DNS response: example.com A, with the
// name of the answer a compression pointer back to the question.
const capturedFrame = "aabbccddeeff0011223344550800450000491a2b4000281100005db8d822c0a80114" +
	"0035c9a200358b1a123481800001000100000000076578616d706c6503636f6d0000010001" +
	"c00c0001000100000e1000045db8d822"

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("bad hex in test: %v", err)
	}
	return b
}

func TestDecodeDNSQuery(t *testing.T) {
	msg := dnsMessage(0x1234, 0x0100, []dnsQuestion{{name: "example.com", typ: 1, class: 1}}, nil)
	flow := decode(v4Frame(numUDP, udpSegment(52114, dnsPort, msg)))

	if flow.Proto != protoDNS {
		t.Errorf("Proto = %q, want DNS", flow.Proto)
	}
	if flow.Info != "Standard query A example.com" {
		t.Errorf("Info = %q", flow.Info)
	}
	if flow.Host != testServerIP+":53" {
		t.Errorf("Host = %q", flow.Host)
	}
	if got := field(t, flow, layerDNS, "Transaction ID"); got != "0x1234" {
		t.Errorf("Transaction ID = %q", got)
	}
	if got := field(t, flow, layerDNS, "Flags"); got != "Standard query, recursion desired" {
		t.Errorf("Flags = %q", got)
	}
	if got := field(t, flow, layerDNS, "Question"); got != "A example.com IN" {
		t.Errorf("Question = %q", got)
	}
}

func TestDecodeMDNSQuery(t *testing.T) {
	questions := []dnsQuestion{
		{name: "example.com", typ: 1, class: 1},
		{name: "www.example.com", typ: 28, class: 1},
	}
	msg := dnsMessage(0, 0, questions, nil)
	flow := decode(v4Frame(numUDP, udpSegment(mdnsPort, mdnsPort, msg)))

	if flow.Proto != protoDNS {
		t.Errorf("Proto = %q, want DNS", flow.Proto)
	}
	if flow.Info != "Standard query A example.com AAAA www.example.com" {
		t.Errorf("Info = %q", flow.Info)
	}
	if got := field(t, flow, layerDNS, "Question"); got != "A example.com IN" {
		t.Errorf("first Question = %q", got)
	}
}

func TestDecodeDNSResponse(t *testing.T) {
	answers := []dnsAnswer{
		{name: "example.com", typ: 5, class: 1, ttl: 300, data: dnsName("cdn.example.net")},
		{name: "cdn.example.net", typ: 1, class: 1, ttl: 60, data: []byte{93, 184, 216, 34}},
	}
	msg := dnsMessage(0xbeef, 0x8180, []dnsQuestion{{name: "example.com", typ: 1, class: 1}}, answers)
	flow := decode(v4Frame(numUDP, udpSegment(dnsPort, 40000, msg)))

	if flow.Info != "Standard query response A example.com" {
		t.Errorf("Info = %q", flow.Info)
	}
	if got := field(t, flow, layerDNS, "Answer"); got != "CNAME example.com cdn.example.net" {
		t.Errorf("first Answer = %q", got)
	}
	if got := field(t, flow, layerDNS, "Flags"); got != "Standard query, response, recursion desired, recursion available" {
		t.Errorf("Flags = %q", got)
	}
}

func TestDecodeCapturedDNSResponse(t *testing.T) {
	flow := decode(mustHex(t, capturedFrame))

	if flow.Proto != protoDNS {
		t.Errorf("Proto = %q, want DNS", flow.Proto)
	}
	if flow.Info != "Standard query response A example.com" {
		t.Errorf("Info = %q", flow.Info)
	}
	if flow.Host != "192.168.1.20:51618" {
		t.Errorf("Host = %q", flow.Host)
	}
	if got := field(t, flow, layerDNS, "Transaction ID"); got != "0x1234" {
		t.Errorf("Transaction ID = %q", got)
	}
	if got := field(t, flow, layerDNS, "Answer"); got != "A example.com 93.184.216.34" {
		t.Errorf("Answer = %q", got)
	}
}

func TestDecodeDNSOverTCP(t *testing.T) {
	msg := dnsMessage(0x1234, 0x0100, []dnsQuestion{{name: "example.com", typ: 1, class: 1}}, nil)
	seg := tcpSegment(tcpHeader{srcPort: 51000, dstPort: dnsPort, flags: tcpFlagPSH | tcpFlagACK}, tcpDNS(msg, uint16(len(msg))))
	flow := decode(v4Frame(numTCP, seg))

	if flow.Proto != protoDNS {
		t.Errorf("Proto = %q, want DNS", flow.Proto)
	}
	if flow.Info != "Standard query A example.com" {
		t.Errorf("Info = %q", flow.Info)
	}
}

func TestDecodeDNSOverTCPPartialMessage(t *testing.T) {
	msg := dnsMessage(0x1234, 0x0100, []dnsQuestion{{name: "example.com", typ: 1, class: 1}}, nil)
	seg := tcpSegment(tcpHeader{srcPort: 51000, dstPort: dnsPort, flags: tcpFlagACK}, tcpDNS(msg[:10], uint16(len(msg))))
	flow := decode(v4Frame(numTCP, seg))

	if flow.Proto != protoTCP {
		t.Errorf("Proto = %q, want TCP until the whole message is here", flow.Proto)
	}
	if hasLayer(flow, layerDNS) {
		t.Errorf("a partial DNS message was decoded: %v", layerNames(flow))
	}
	if flow.Info != "51000 → 53 [ACK] Len=12" {
		t.Errorf("Info = %q", flow.Info)
	}
}

func TestDecodeDNSBroken(t *testing.T) {
	short := udpSegment(52114, dnsPort, []byte("abc"))
	questions := make([]byte, dnsHeaderLen)
	questions[4] = 2 // two questions announced, none present

	for _, c := range []struct {
		name  string
		frame []byte
		info  string
	}{
		{"cut before the header", v4Frame(numUDP, short), "[truncated] DNS message"},
		{"questions missing", v4Frame(numUDP, udpSegment(52114, dnsPort, questions)), "Malformed DNS message"},
	} {
		t.Run(c.name, func(t *testing.T) {
			flow := decode(c.frame)
			if flow.Proto != protoDNS {
				t.Errorf("Proto = %q, want DNS", flow.Proto)
			}
			if flow.Info != c.info {
				t.Errorf("Info = %q, want %q", flow.Info, c.info)
			}
		})
	}
}
