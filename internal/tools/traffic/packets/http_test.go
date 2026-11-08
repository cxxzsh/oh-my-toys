package packets

import (
	"strings"
	"testing"
)

func TestDecodeHTTPRequest(t *testing.T) {
	head := "GET /index.html HTTP/1.1\r\nHost: example.com\r\nUser-Agent: omt-test\r\n\r\nbody"
	flow := decode(v4Frame(numTCP, tcpSegment(tcpHeader{
		srcPort: 52114, dstPort: 80, flags: tcpFlagPSH | tcpFlagACK,
	}, []byte(head))))

	if flow.Proto != protoHTTP {
		t.Errorf("Proto = %q, want HTTP", flow.Proto)
	}
	if flow.Method != "GET" || flow.Path != "/index.html" {
		t.Errorf("Method = %q, Path = %q", flow.Method, flow.Path)
	}
	if flow.Status != 0 {
		t.Errorf("Status = %d, want 0 for a request", flow.Status)
	}
	if flow.Info != "GET /index.html HTTP/1.1" {
		t.Errorf("Info = %q", flow.Info)
	}
	if got := field(t, flow, layerHTTP, "Request line"); got != "GET /index.html HTTP/1.1" {
		t.Errorf("Request line = %q", got)
	}
	if got := field(t, flow, layerHTTP, "Host"); got != "example.com" {
		t.Errorf("Host header = %q", got)
	}
	if got := field(t, flow, layerHTTP, "User-Agent"); got != "omt-test" {
		t.Errorf("User-Agent = %q", got)
	}
}

func TestDecodeHTTPRequestWithQuery(t *testing.T) {
	head := "POST /api/items?x=1 HTTP/1.1\r\nContent-Type: application/json\r\nContent-Length: 2\r\n\r\n{}"
	flow := decode(v4Frame(numTCP, tcpSegment(tcpHeader{srcPort: 40000, dstPort: 8080}, []byte(head))))
	if flow.Method != "POST" || flow.Path != "/api/items?x=1" {
		t.Errorf("Method = %q, Path = %q", flow.Method, flow.Path)
	}
	if got := field(t, flow, layerHTTP, "Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q", got)
	}
}

func TestDecodeHTTPResponse(t *testing.T) {
	head := "HTTP/1.1 200 OK\r\nContent-Type: text/html\r\n\r\n<html></html>"
	flow := decode(v4Frame(numTCP, tcpSegment(tcpHeader{srcPort: 80, dstPort: 52114}, []byte(head))))

	if flow.Proto != protoHTTP {
		t.Errorf("Proto = %q, want HTTP", flow.Proto)
	}
	if flow.Status != 200 {
		t.Errorf("Status = %d, want 200", flow.Status)
	}
	if flow.Method != "" || flow.Path != "" {
		t.Errorf("a response keeps no request fields, got %q %q", flow.Method, flow.Path)
	}
	if flow.Info != "HTTP/1.1 200 OK" {
		t.Errorf("Info = %q", flow.Info)
	}
	if got := field(t, flow, layerHTTP, "Status line"); got != "HTTP/1.1 200 OK" {
		t.Errorf("Status line = %q", got)
	}
}

func TestDecodeHTTPBareLineFeed(t *testing.T) {
	flow := decode(v4Frame(numTCP, tcpSegment(tcpHeader{srcPort: 1, dstPort: 2}, []byte("HTTP/1.1 404\r\n\r\n"))))
	if flow.Status != 404 {
		t.Errorf("Status = %d, want 404", flow.Status)
	}
	if flow.Info != "HTTP/1.1 404" {
		t.Errorf("Info = %q", flow.Info)
	}

	flow = decode(v4Frame(numTCP, tcpSegment(tcpHeader{srcPort: 1, dstPort: 2}, []byte("GET /a HTTP/1.1\nHost: a\n\n"))))
	if flow.Method != "GET" || flow.Path != "/a" {
		t.Errorf("Method = %q, Path = %q", flow.Method, flow.Path)
	}
	if got := field(t, flow, layerHTTP, "Host"); got != "a" {
		t.Errorf("Host = %q", got)
	}
}

func TestDecodeHTTPTruncated(t *testing.T) {
	flow := decode(v4Frame(numTCP, tcpSegment(tcpHeader{srcPort: 1, dstPort: 80}, []byte("GET /index.ht"))))
	if flow.Proto != protoHTTP {
		t.Errorf("Proto = %q, want HTTP from the method token", flow.Proto)
	}
	if flow.Info != "[truncated] HTTP start line" {
		t.Errorf("Info = %q", flow.Info)
	}
	if flow.Method != "" {
		t.Errorf("Method = %q, want none from a cut line", flow.Method)
	}
}

func TestDecodeHTTPHeadersCutShort(t *testing.T) {
	head := "GET / HTTP/1.1\r\nHost: example.com\r\nX-Cut: va"
	flow := decode(v4Frame(numTCP, tcpSegment(tcpHeader{srcPort: 1, dstPort: 80}, []byte(head))))
	if flow.Proto != protoHTTP {
		t.Errorf("Proto = %q, want HTTP", flow.Proto)
	}
	if !strings.HasPrefix(flow.Info, "GET / HTTP/1.1") {
		t.Errorf("Info = %q", flow.Info)
	}
	if got := field(t, flow, layerHTTP, "Host"); got != "example.com" {
		t.Errorf("Host = %q", got)
	}
	http := findLayer(t, flow, layerHTTP)
	if len(http.Fields) != 2 {
		t.Errorf("fields = %v, want the start line and one whole header", http.Fields)
	}
}

func TestDecodeHTTPStatusWithoutReason(t *testing.T) {
	flow := decode(v4Frame(numTCP, tcpSegment(tcpHeader{srcPort: 1, dstPort: 2}, []byte("HTTP/1.1 abc\r\n\r\n"))))
	if flow.Status != 0 {
		t.Errorf("Status = %d, want 0 for a status line without a code", flow.Status)
	}
	if flow.Proto != protoHTTP {
		t.Errorf("Proto = %q, want HTTP", flow.Proto)
	}
}

func TestDecodeNonHTTPPayload(t *testing.T) {
	for _, c := range []struct {
		name string
		data []byte
	}{
		{name: "binary", data: []byte{1, 2, 3}},
		{name: "method letters without a space", data: []byte("GETAWAY")},
		{name: "cut method", data: []byte("GET")},
	} {
		t.Run(c.name, func(t *testing.T) {
			flow := decode(v4Frame(numTCP, tcpSegment(tcpHeader{srcPort: 1, dstPort: 2}, c.data)))
			if flow.Proto != protoTCP {
				t.Errorf("Proto = %q, want TCP", flow.Proto)
			}
			if hasLayer(flow, layerHTTP) {
				t.Errorf("payload %q became HTTP: %v", c.data, layerNames(flow))
			}
		})
	}
}
