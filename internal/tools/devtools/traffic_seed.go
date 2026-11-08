package devtools

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// The media types, protocols and hosts the made-up capture uses, named
// once so that the canned data reads as what it is.
const (
	mediaJSON = "application/json"
	mediaHTML = "text/html"
	mediaPNG  = "image/png"
	mediaForm = "application/x-www-form-urlencoded"
	mediaText = "text/plain"
	protoTCP  = "TCP"
	protoHTTP = "HTTP/1.1"
	methodGET = "GET"
	tls13     = "TLS 1.3"

	// The header names and values the canned packets share.
	fieldAccept  = "Accept"
	fieldDest    = "Destination"
	fieldSource  = "Source"
	fieldType    = "Type"
	fieldProto   = "Protocol"
	fieldSrcPort = "Source Port"
	fieldDstPort = "Destination Port"
	fieldFlags   = "Flags"
	fieldUpgrade = "Upgrade"
	layerIPv4    = "IPv4"
	hostTLS      = "93.184.216.34:443"
	ipClient     = "192.168.1.10"
)

// How much the made-up capture holds: enough rows to scroll, and to see
// that the list only draws what is in view.
const (
	cannedFlows   = 240
	cannedPackets = 60
)

// exampleRules are the rules the made-up rule book starts with: one of
// each kind the inspector draws differently.
func exampleRules() []tool.Rule {
	return []tool.Rule{
		{
			ID: "rule-1", Name: "Add a debug header", Enabled: true, Kind: tool.RuleRewrite,
			Match:   tool.RuleMatch{Pattern: "https://api.example.com/*", Methods: []string{methodGET}},
			Rewrite: tool.RewriteEffect{RequestHeaders: []tool.HeaderEdit{{Name: "X-Debug", Value: "1"}}},
		},
		{
			ID: "rule-2", Name: "Mock the items page", Enabled: true, Kind: tool.RuleMock,
			Match: tool.RuleMatch{Pattern: "https://api.example.com/v1/items*", Methods: []string{methodGET}},
			Mock: tool.MockEffect{
				Status:  200,
				Headers: []tool.Header{{Name: "Content-Type", Value: mediaJSON}},
				Body:    `{"items":[],"mocked":true}`,
			},
		},
		{
			ID: "rule-3", Name: "Slow the stylesheet", Enabled: true, Kind: tool.RuleDelay,
			Match: tool.RuleMatch{Pattern: "*://cdn.example.com/*.css"},
			Delay: 300 * time.Millisecond,
		},
	}
}

// replayOf is the canned flow the made-up replays repeat.
const replayOf = 1

// fakeFingerprint is the made-up certificate's SHA-256, written the way
// the real one is.
const fakeFingerprint = "7b3f1c0e5a94d2c6817f0a2d4e6b8c93a1f5d7e90b2c4a6d8e0f1a3b5c7d9e2f"

// fakeCertPath is where the made-up certificate claims its PEM file is.
// Nothing is written there.
func fakeCertPath() string {
	return filepath.Join(os.TempDir(), "oh-my-toys-dev", "traffic-ca.pem")
}

// bodyJSON is a response body that the inspector shows pretty-printed.
const bodyJSON = `{"id":42,"name":"Widget","tags":["tools","mac"],"price":12.5,"inStock":true}`

// bodyHTML is a response body that is shown as text.
const bodyHTML = `<!doctype html><html lang="en"><head><title>Example</title></head>` +
	`<body><h1>Example Domain</h1><p>This domain is for use in examples.</p></body></html>`

// bodyForm is a request body that is decoded to its pairs.
const bodyForm = "user=ada&password=hunter2&remember=1"

// seedTraffic fills both logs with the made-up capture.
func seedTraffic(proxy, packets *fakeLog) {
	now := time.Now()
	for i := range cannedFlows {
		proxy.add(cannedFlow(i, now))
	}
	for i := range cannedPackets {
		packets.add(cannedPacket(i, now))
	}
}

// cannedFlow is one of the flows the made-up capture holds: every kind in
// turn, so the list, the filters and the detail all have something to show.
func cannedFlow(i int, now time.Time) tool.Flow {
	started := now.Add(-time.Duration(cannedFlows-i) * 900 * time.Millisecond)
	switch i % 12 {
	case 0:
		return jsonFlow(started, i)
	case 1:
		return postFlow(started)
	case 2:
		return htmlFlow(started, i)
	case 3:
		return imageFlow(started)
	case 4:
		return redirectFlow(started)
	case 5:
		return notFoundFlow(started)
	case 6:
		return serverErrorFlow(started)
	case 7:
		return failedFlow(started)
	case 8:
		return blockedFlow(started)
	case 9:
		return tunnelFlow(started)
	case 10:
		return websocketFlow(started)
	}
	return replayFlow(started)
}

// flowRow is the small part of a flow the list shows.
func flowRow(started time.Time, source tool.FlowSource, method, host, path, proto string) tool.FlowRow {
	return tool.FlowRow{Source: source, Started: started, Method: method, Host: host, Path: path, Proto: proto}
}

// proxyFlow is a proxied flow with what both sides carried.
func proxyFlow(row tool.FlowRow, url string, req, resp tool.HTTPMessage) tool.Flow {
	return tool.Flow{FlowRow: row, URL: url, Request: req, Response: resp}
}

// message is one side of a flow, with a media type and a body.
func message(media string, data string, headers ...tool.Header) tool.HTTPMessage {
	h := append([]tool.Header{{Name: "Content-Type", Value: media}}, headers...)
	return tool.HTTPMessage{
		Headers: h,
		Body:    tool.Body{Type: media, Data: []byte(data), Size: int64(len(data))},
	}
}

// jsonFlow is a GET that answered JSON.
func jsonFlow(started time.Time, i int) tool.Flow {
	row := flowRow(started, tool.SourceProxy, methodGET, "api.example.com", fmt.Sprintf("/v1/items?page=%d", i%7+1), "h2")
	row.Status, row.Type, row.State = 200, mediaJSON, tool.FlowDone
	row.Size, row.Duration = int64(len(bodyJSON)), 34*time.Millisecond
	f := proxyFlow(row, "https://"+row.Host+row.Path,
		message(mediaJSON, "", tool.Header{Name: fieldAccept, Value: mediaJSON}),
		message(mediaJSON, bodyJSON, tool.Header{Name: "Server", Value: "envoy"}))
	f.Decrypted, f.TLSVersion, f.ServerAddr = true, tls13, hostTLS
	return f
}

// postFlow is a POST that created something, with a form body.
func postFlow(started time.Time) tool.Flow {
	row := flowRow(started, tool.SourceProxy, "POST", "api.example.com", "/v1/orders", "HTTP/1.1")
	row.Status, row.Type, row.State = 201, mediaJSON, tool.FlowDone
	row.Size, row.Duration = int64(len(bodyJSON)), 118*time.Millisecond
	f := proxyFlow(row, "https://"+row.Host+row.Path,
		message(mediaForm, bodyForm),
		message(mediaJSON, `{"id":77,"status":"created"}`))
	f.Decrypted, f.TLSVersion, f.ServerAddr = true, tls13, hostTLS
	return f
}

// htmlFlow is a page that answered HTML.
func htmlFlow(started time.Time, i int) tool.Flow {
	row := flowRow(started, tool.SourceProxy, methodGET, "www.example.com", fmt.Sprintf("/docs/%d", i), "h2")
	row.Status, row.Type, row.State = 200, mediaHTML, tool.FlowDone
	row.Size, row.Duration = int64(len(bodyHTML)), 62*time.Millisecond
	f := proxyFlow(row, "https://"+row.Host+row.Path,
		message(mediaHTML, "", tool.Header{Name: fieldAccept, Value: mediaHTML + ",*/*"}),
		message(mediaHTML, bodyHTML, tool.Header{Name: "Cache-Control", Value: "max-age=600"}))
	f.Decrypted, f.TLSVersion, f.ServerAddr = true, "TLS 1.2", "93.184.215.14:443"
	return f
}

// imageFlow is a GET that answered a PNG.
func imageFlow(started time.Time) tool.Flow {
	png := append([]byte("\x89PNG\r\n\x1a\n"), bytesOf(0x00, 0x0d, 0x49, 0x48, 0x44, 0x52)...)
	png = append(png, bytesOf(0xde, 0xad, 0xbe, 0xef, 0x00, 0x01, 0x02, 0x03)...)
	row := flowRow(started, tool.SourceProxy, methodGET, "cdn.example.com", "/img/logo.png", "h2")
	row.Status, row.Type, row.State = 200, mediaPNG, tool.FlowDone
	row.Size, row.Duration = int64(len(png)), 21*time.Millisecond
	f := proxyFlow(row, "https://"+row.Host+row.Path,
		tool.HTTPMessage{Headers: []tool.Header{{Name: fieldAccept, Value: "image/*"}}},
		message(mediaPNG, string(png)))
	f.Decrypted, f.TLSVersion, f.ServerAddr = true, tls13, "151.101.1.44:443"
	return f
}

// redirectFlow answered 301, which the page shows as information.
func redirectFlow(started time.Time) tool.Flow {
	row := flowRow(started, tool.SourceProxy, methodGET, "example.com", "/old", "HTTP/1.1")
	row.Status, row.Type, row.State = 301, mediaHTML, tool.FlowDone
	row.Size, row.Duration = 0, 8*time.Millisecond
	f := proxyFlow(row, "http://"+row.Host+row.Path, tool.HTTPMessage{},
		tool.HTTPMessage{Headers: []tool.Header{{Name: "Location", Value: "https://example.com/new"}}})
	f.ServerAddr = "93.184.216.34:80"
	return f
}

// notFoundFlow answered 404, a warning.
func notFoundFlow(started time.Time) tool.Flow {
	row := flowRow(started, tool.SourceProxy, methodGET, "api.example.com", "/v1/missing", "h2")
	row.Status, row.Type, row.State = 404, mediaJSON, tool.FlowDone
	row.Size, row.Duration = 38, 12*time.Millisecond
	f := proxyFlow(row, "https://"+row.Host+row.Path, tool.HTTPMessage{},
		message(mediaJSON, `{"error":"not found","code":"item_missing"}`))
	f.Decrypted, f.TLSVersion, f.ServerAddr = true, tls13, hostTLS
	return f
}

// serverErrorFlow answered 500, a fault.
func serverErrorFlow(started time.Time) tool.Flow {
	row := flowRow(started, tool.SourceProxy, "POST", "api.example.com", "/v1/login", "h2")
	row.Status, row.Type, row.State = 500, mediaText, tool.FlowDone
	row.Size, row.Duration = 27, 2400*time.Millisecond
	f := proxyFlow(row, "https://"+row.Host+row.Path,
		message(mediaJSON, `{"user":"ada","password":"hunter2"}`),
		message(mediaText, "internal error"))
	f.Decrypted, f.TLSVersion, f.ServerAddr = true, tls13, hostTLS
	return f
}

// failedFlow never reached its server.
func failedFlow(started time.Time) tool.Flow {
	row := flowRow(started, tool.SourceProxy, methodGET, "down.example.com", "/health", "h2")
	row.State, row.Info = tool.FlowFailed, "dial tcp 10.0.0.9:443: connection refused"
	row.Duration = 3002 * time.Millisecond
	f := proxyFlow(row, "https://"+row.Host+row.Path, tool.HTTPMessage{}, tool.HTTPMessage{})
	f.Decrypted, f.ServerAddr = true, "10.0.0.9:443"
	return f
}

// blockedFlow was closed by a rule.
func blockedFlow(started time.Time) tool.Flow {
	row := flowRow(started, tool.SourceProxy, methodGET, "ads.example.net", "/track?u=42", "HTTP/1.1")
	row.State, row.Ruled = tool.FlowBlocked, true
	row.Info, row.Duration = "closed by a rule", time.Millisecond
	f := proxyFlow(row, "https://"+row.Host+row.Path, tool.HTTPMessage{}, tool.HTTPMessage{})
	f.RuleHits, f.Decrypted, f.ServerAddr = []string{"Block ads"}, true, "203.0.113.7:443"
	return f
}

// tunnelFlow is an HTTPS connection the capture did not open, which is one
// row and no content.
func tunnelFlow(started time.Time) tool.Flow {
	row := flowRow(started, tool.SourceProxy, "CONNECT", "github.com:443", "", "tunnel")
	row.State, row.Duration, row.Size = tool.FlowOpen, 420*time.Millisecond, 8192
	f := proxyFlow(row, "https://github.com:443", tool.HTTPMessage{}, tool.HTTPMessage{})
	f.ServerAddr = "140.82.121.4:443"
	return f
}

// replayFlow is a request sent again, which says which flow it repeats:
// the first of the canned flows.
func replayFlow(started time.Time) tool.Flow {
	f := jsonFlow(started, 0)
	f.Path, f.Method, f.ReplayOf = "/v1/items?page=replay", methodGET, replayOf
	f.URL = "https://" + f.Host + f.Path
	return f
}

// websocketFlow is an upgrade, recorded as a request and then tunnelled.
func websocketFlow(started time.Time) tool.Flow {
	row := flowRow(started, tool.SourceProxy, methodGET, "ws.example.com", "/socket", "HTTP/1.1")
	row.Status, row.State, row.Info = 101, tool.FlowOpen, "WebSocket upgrade"
	row.Duration, row.Size = 12*time.Millisecond, 0
	f := proxyFlow(row, "https://"+row.Host+row.Path,
		tool.HTTPMessage{Headers: []tool.Header{
			{Name: fieldUpgrade, Value: "websocket"},
			{Name: "Connection", Value: fieldUpgrade},
			{Name: "Sec-WebSocket-Key", Value: "dGhlIHNhbXBsZSBub25jZQ=="},
		}},
		tool.HTTPMessage{Headers: []tool.Header{
			{Name: fieldUpgrade, Value: "websocket"},
			{Name: "Sec-WebSocket-Accept", Value: "s3pPLMBiTxaQ9kYGzzhZRbK+xOo="},
		}})
	f.Decrypted, f.TLSVersion, f.ServerAddr = true, tls13, hostTLS
	return f
}

// liveFlowAt is a flow recorded while the made-up capture runs, so that a
// page can watch the list follow its end.
func liveFlowAt(now time.Time, n int) tool.Flow {
	f := jsonFlow(now, n)
	f.Path = fmt.Sprintf("/v1/items?page=%d&live=1", n+1)
	f.URL = "https://" + f.Host + f.Path
	f.Duration = time.Duration(10+n%90) * time.Millisecond
	return f
}

// cannedPacket is one of the packets the made-up capture holds.
func cannedPacket(i int, now time.Time) tool.Flow {
	at := now.Add(-time.Duration(cannedPackets-i) * 700 * time.Millisecond)
	switch i % 6 {
	case 0:
		return synPacket(at, i)
	case 1:
		return dnsPacket(at)
	case 2:
		return tlsPacket(at)
	case 3:
		return httpPacket(at)
	case 4:
		return arpPacket(at)
	}
	return icmpPacket(at)
}

// packetAt is a packet recorded while the made-up capture runs.
func packetAt(now time.Time, n int) tool.Flow { return synPacket(now, n) }

// packetFlow wraps layers and bytes as a packet row.
func packetFlow(at time.Time, proto, host, info string, layers []tool.Layer, raw []byte) tool.Flow {
	row := flowRow(at, tool.SourcePacket, "", host, "", proto)
	row.Info, row.State, row.Size = info, tool.FlowDone, int64(len(raw))
	return tool.Flow{FlowRow: row, Packet: &tool.PacketDetail{Layers: layers, Raw: raw}}
}

// synPacket is the first packet of a TCP connection.
func synPacket(at time.Time, i int) tool.Flow {
	port := 52000 + i
	raw := bytesOf(0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x08, 0x00,
		0x45, 0x00, 0x00, 0x3c, 0x1c, 0x46, 0x40, 0x00,
		0x40, 0x06, 0xb1, 0xe6, 0xc0, 0xa8, 0x01, 0x0a,
		0x5d, 0xb8, 0xd8, 0x22, 0xcb, 0x20, 0x01, 0xbb)
	return packetFlow(at, protoTCP, "93.184.216.34", fmt.Sprintf("SYN %d → 443", port), []tool.Layer{
		{Name: "Ethernet II", Fields: []tool.Header{
			{Name: fieldDest, Value: "aa:bb:cc:dd:ee:ff"},
			{Name: fieldSource, Value: "11:22:33:44:55:66"},
			{Name: fieldType, Value: "IPv4 (0x0800)"},
		}},
		{Name: layerIPv4, Fields: []tool.Header{
			{Name: fieldSource, Value: ipClient},
			{Name: fieldDest, Value: "93.184.216.34"},
			{Name: "TTL", Value: "64"},
			{Name: fieldProto, Value: "TCP (6)"},
		}},
		{Name: protoTCP, Fields: []tool.Header{
			{Name: fieldSrcPort, Value: fmt.Sprint(port)},
			{Name: fieldDstPort, Value: "443"},
			{Name: fieldFlags, Value: "SYN"},
			{Name: "Window", Value: "65535"},
		}},
	}, raw)
}

// dnsPacket asks for an address.
func dnsPacket(at time.Time) tool.Flow {
	raw := bytesOf(0x45, 0x00, 0x00, 0x3a, 0x00, 0x01, 0x00, 0x00, 0x40, 0x11,
		0x8f, 0x2c, 0xc0, 0xa8, 0x01, 0x0a, 0xc0, 0xa8, 0x01, 0x01, 0x30, 0x39, 0x00, 0x35)
	return packetFlow(at, "DNS", "8.8.8.8", "A api.example.com", []tool.Layer{
		{Name: layerIPv4, Fields: []tool.Header{
			{Name: fieldSource, Value: ipClient},
			{Name: fieldDest, Value: "8.8.8.8"},
			{Name: fieldProto, Value: "UDP (17)"},
		}},
		{Name: "UDP", Fields: []tool.Header{
			{Name: fieldSrcPort, Value: "53421"},
			{Name: fieldDstPort, Value: "53"},
			{Name: "Length", Value: "45"},
		}},
		{Name: "DNS", Fields: []tool.Header{
			{Name: "Transaction ID", Value: "0x1a2b"},
			{Name: "Questions", Value: "1"},
			{Name: "Query", Value: "api.example.com A IN"},
		}},
	}, raw)
}

// tlsPacket is a hello that names the server.
func tlsPacket(at time.Time) tool.Flow {
	raw := bytesOf(0x16, 0x03, 0x01, 0x02, 0x00, 0x01, 0x00, 0x01, 0xfc, 0x03, 0x03)
	return packetFlow(at, "TLS", "93.184.215.14", "ClientHello, server_name=www.example.com", []tool.Layer{
		{Name: protoTCP, Fields: []tool.Header{
			{Name: fieldSrcPort, Value: "53422"},
			{Name: fieldDstPort, Value: "443"},
			{Name: fieldFlags, Value: "PSH, ACK"},
		}},
		{Name: "TLS", Fields: []tool.Header{
			{Name: "Record", Value: "Handshake (22)"},
			{Name: "Version", Value: "TLS 1.0 (0x0301)"},
			{Name: "Handshake", Value: "ClientHello (1)"},
			{Name: "Server Name", Value: "www.example.com"},
			{Name: "ALPN", Value: "h2, http/1.1"},
		}},
	}, raw)
}

// httpPacket is the first line of a plain request.
func httpPacket(at time.Time) tool.Flow {
	raw := []byte("GET /index.html HTTP/1.1\r\nHost: example.com\r\n\r\n")
	return packetFlow(at, "HTTP", "example.com", "GET /index.html", []tool.Layer{
		{Name: protoTCP, Fields: []tool.Header{
			{Name: fieldSrcPort, Value: "53423"},
			{Name: fieldDstPort, Value: "80"},
			{Name: fieldFlags, Value: "PSH, ACK"},
		}},
		{Name: "HTTP", Fields: []tool.Header{
			{Name: "Request", Value: "GET /index.html HTTP/1.1"},
			{Name: "Host", Value: "example.com"},
		}},
	}, raw)
}

// arpPacket asks who holds an address.
func arpPacket(at time.Time) tool.Flow {
	raw := bytesOf(0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x08, 0x06, 0x00, 0x01,
		0x08, 0x00, 0x06, 0x04, 0x00, 0x01)
	return packetFlow(at, "ARP", "192.168.1.1", "who has 192.168.1.1? tell 192.168.1.10", []tool.Layer{
		{Name: "Ethernet II", Fields: []tool.Header{
			{Name: fieldDest, Value: "ff:ff:ff:ff:ff:ff"},
			{Name: fieldType, Value: "ARP (0x0806)"},
		}},
		{Name: "ARP", Fields: []tool.Header{
			{Name: "Operation", Value: "request (1)"},
			{Name: "Sender", Value: ipClient},
			{Name: "Target", Value: "192.168.1.1"},
		}},
	}, raw)
}

// icmpPacket is a ping.
func icmpPacket(at time.Time) tool.Flow {
	raw := bytesOf(0x45, 0x00, 0x00, 0x54, 0x00, 0x01, 0x00, 0x00, 0x40, 0x01,
		0x00, 0x00, 0xc0, 0xa8, 0x01, 0x0a, 0x08, 0x08, 0x08, 0x08, 0x08, 0x00)
	return packetFlow(at, "ICMP", "8.8.8.8", "echo request, seq 1", []tool.Layer{
		{Name: layerIPv4, Fields: []tool.Header{
			{Name: fieldSource, Value: ipClient},
			{Name: fieldDest, Value: "8.8.8.8"},
			{Name: fieldProto, Value: "ICMP (1)"},
		}},
		{Name: "ICMP", Fields: []tool.Header{
			{Name: fieldType, Value: "Echo request (8)"},
			{Name: "Code", Value: "0"},
			{Name: "Checksum", Value: "0x0000"},
		}},
	}, raw)
}

// bytesOf is a byte slice of the values given.
func bytesOf(values ...byte) []byte { return values }
