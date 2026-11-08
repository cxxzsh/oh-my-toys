package packets

import "testing"

// A real ClientHello captured on the wire: TLS 1.3 offered over a TLS 1.0
// record, with the SNI example.com and the ALPN protocols h2 and http/1.1.
const capturedClientHello = "1603010086010000820303000102030405060708090a0b0c0d0e0f1011121314" +
	"15161718191a1b1c1d1e1f20aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" +
	"aaaaaaaaaaaaaaaaaaaaaaaa0008130113021303c02b0100003100000010000e" +
	"00000b6578616d706c652e636f6d0010000e000c02683208687474702f312e31" +
	"002b000706030403030302"

func TestDecodeCapturedClientHello(t *testing.T) {
	hello := mustHex(t, capturedClientHello)
	seg := tcpSegment(tcpHeader{srcPort: 52114, dstPort: 443, flags: tcpFlagPSH | tcpFlagACK}, hello)
	flow := decode(v4Frame(numTCP, seg))

	if flow.Proto != protoTLS {
		t.Errorf("Proto = %q, want TLS", flow.Proto)
	}
	if flow.Info != "Client Hello (SNI example.com)" {
		t.Errorf("Info = %q", flow.Info)
	}
	if got := field(t, flow, layerTLS, "Record Type"); got != "Handshake (22)" {
		t.Errorf("Record Type = %q", got)
	}
	if got := field(t, flow, layerTLS, "Record Version"); got != "TLS 1.0" {
		t.Errorf("Record Version = %q", got)
	}
	if got := field(t, flow, layerTLS, "Handshake Type"); got != "ClientHello" {
		t.Errorf("Handshake Type = %q", got)
	}
	if got := field(t, flow, layerTLS, "Version"); got != "TLS 1.2" {
		t.Errorf("Version = %q", got)
	}
	if got := field(t, flow, layerTLS, "SNI"); got != "example.com" {
		t.Errorf("SNI = %q", got)
	}
	if got := field(t, flow, layerTLS, "ALPN"); got != "h2, http/1.1" {
		t.Errorf("ALPN = %q", got)
	}
}

func TestDecodeClientHelloWithoutNames(t *testing.T) {
	hello := tlsHello(0x0303, "", nil)
	flow := decode(v4Frame(numTCP, tcpSegment(tcpHeader{srcPort: 1, dstPort: 443}, hello)))
	if flow.Info != "Client Hello" {
		t.Errorf("Info = %q", flow.Info)
	}
	if got := field(t, flow, layerTLS, "Version"); got != "TLS 1.2" {
		t.Errorf("Version = %q", got)
	}
}

func TestDecodeTLSRecords(t *testing.T) {
	for _, c := range []struct {
		name  string
		body  []byte
		info  string
		proto string
	}{
		{
			name: "application data", body: []byte{23, 3, 3, 0, 5, 1, 2, 3, 4, 5},
			info: "Application Data (23), 5 bytes", proto: protoTLS,
		},
		{
			name: "server hello", body: []byte{22, 3, 3, 0, 14, 2, 0, 0, 10, 3, 3, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			info: "TLS handshake", proto: protoTLS,
		},
		{
			name: "hello cut short", body: []byte{22, 3, 3, 0, 8, 1, 0, 0, 64, 3, 3, 0, 1},
			info: "TLS handshake", proto: protoTLS,
		},
		{
			name: "record cut short", body: []byte{22, 3, 3, 0, 200, 1, 2, 3},
			info: "Handshake (22) [truncated]", proto: protoTLS,
		},
		{
			name: "not a TLS version", body: []byte{22, 2, 3, 0, 0},
			info: "50000 → 443 [ACK] Len=5", proto: protoTCP,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			seg := tcpSegment(tcpHeader{srcPort: 50000, dstPort: 443, flags: tcpFlagACK}, c.body)
			flow := decode(v4Frame(numTCP, seg))
			if flow.Proto != c.proto {
				t.Errorf("Proto = %q, want %q", flow.Proto, c.proto)
			}
			if flow.Info != c.info {
				t.Errorf("Info = %q, want %q", flow.Info, c.info)
			}
		})
	}
}
