package packets

import (
	"encoding/hex"
	"testing"

	"github.com/cxxzsh/oh-my-toys/internal/platform/pcap"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// mustSeed decodes a hex seed of the corpus, which the test author writes.
func mustSeed(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic("bad seed hex: " + err.Error())
	}
	return b
}

// FuzzDecode feeds arbitrary bytes to every link type: decoding a capture
// must never panic, and every packet must come back as a flow that names a
// protocol and says something about itself.
func FuzzDecode(f *testing.F) {
	seeds := [][]byte{
		mustSeed(capturedFrame),
		mustSeed(capturedClientHello),
		mustSeed("aabbccddeeff0011223344550800aabbccddeeff0011223344550800"),
		v4Frame(numTCP, tcpSegment(tcpHeader{srcPort: 1, dstPort: 443}, []byte("GET / HTTP/1.1\r\n\r\n"))),
		v4Frame(numUDP, udpSegment(53, 53, []byte{1, 2, 3})),
		v6Frame(extHopByHop, append(extHeader(numICMPv6, 0), 128, 0, 0, 0, 0, 1, 0, 2)),
		append([]byte{2, 0, 0, 0}, ipv4Packet(ipv4Header{src: testClientIP, dst: testServerIP, proto: numTCP}, nil)...),
		ethFrame("ff:ff:ff:ff:ff:ff", testClientMAC, etherTypeARP,
			arpPacket(arpOpRequest, testClientMAC, testClientIP, "00:00:00:00:00:00", "192.168.1.1")),
		{0x00},
		{},
	}
	for _, seed := range seeds {
		for link := range 3 {
			f.Add(seed, uint8(link))
		}
	}
	f.Fuzz(func(t *testing.T, data []byte, link uint8) {
		for _, l := range []pcap.LinkType{pcap.LinkEthernet, pcap.LinkNull, pcap.LinkRaw, pcap.LinkType(link)} {
			checkDecode(t, l, data)
		}
	})
}

// checkDecode is the invariant every packet must keep, whatever its bytes.
func checkDecode(t *testing.T, link pcap.LinkType, data []byte) {
	t.Helper()
	flow := Decode(pcap.Packet{Time: testTime, Link: link, Data: data, Length: len(data)})
	if flow.Source != tool.SourcePacket || flow.Packet == nil {
		t.Fatalf("link %d: no packet flow for %x", link, data)
	}
	if flow.Proto == "" || flow.Info == "" {
		t.Fatalf("link %d: flow without a protocol or a summary: %+v", link, flow.FlowRow)
	}
	if want := min(len(data), tool.MaxBody); len(flow.Packet.Raw) != want {
		t.Fatalf("link %d: Raw is %d bytes, want %d", link, len(flow.Packet.Raw), want)
	}
}
