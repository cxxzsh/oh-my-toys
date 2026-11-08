package flowlog

import (
	"testing"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// filterLog holds one flow of each shape the filter fields separate.
func filterLog() *Log {
	log := New(20, 1<<20)
	log.Put(tool.Flow{
		FlowRow: tool.FlowRow{
			Source: tool.SourceProxy, Method: "GET", Host: "example.com:443",
			Path: "/index.html", Proto: "HTTP/1.1", Status: 200, Type: "text/html",
			State: tool.FlowDone,
		},
		URL: "https://example.com/index.html",
		Request: tool.HTTPMessage{
			Headers: []tool.Header{
				{Name: "Accept", Value: "text/html"},
				{Name: "X-Trace", Value: "abc123"},
			},
			Body: tool.Body{Type: "text/plain", Data: []byte("hello request")},
		},
		Response: tool.HTTPMessage{Body: tool.Body{Type: "text/html", Data: []byte("<b>hi</b>")}},
	})
	log.Put(tool.Flow{
		FlowRow: tool.FlowRow{
			Source: tool.SourceProxy, Method: "POST", Host: "api.example.com:443",
			Path: "/v1/items", Proto: "HTTP/1.1", Status: 404, Type: "application/json",
			State: tool.FlowDone,
		},
		URL: "https://api.example.com/v1/items",
		Request: tool.HTTPMessage{
			Headers: []tool.Header{{Name: "Content-Type", Value: "application/json"}},
			Body:    tool.Body{Type: "application/json", Data: []byte(`{"name":"widget"}`)},
		},
		Response: tool.HTTPMessage{
			Headers: []tool.Header{{Name: "Content-Type", Value: "application/json"}},
			Body:    tool.Body{Type: "application/json", Data: []byte(`{"error":"Not Found"}`)},
		},
	})
	log.Put(tool.Flow{
		FlowRow: tool.FlowRow{
			Source: tool.SourceProxy, Method: "GET", Host: "cdn.test.net:443",
			Path: "/logo.png", Proto: "HTTP/1.1", Status: 500, Type: "image/png",
			State: tool.FlowFailed, Info: "connection reset",
		},
		URL:      "https://cdn.test.net/logo.png",
		Response: tool.HTTPMessage{Body: tool.Body{Type: "image/png", Data: []byte("needle in bytes")}},
	})
	log.Put(tool.Flow{
		FlowRow: tool.FlowRow{
			Source: tool.SourceProxy, Method: "DELETE", Host: "example.com:443",
			Path: "/item/7", Proto: "HTTP/1.1", State: tool.FlowOpen,
		},
		URL: "https://example.com/item/7",
	})
	log.Put(tool.Flow{
		FlowRow: tool.FlowRow{
			Source: tool.SourceProxy, Method: "GET", Host: "ads.example.com:443",
			Path: "/banner", Proto: "HTTP/1.1", State: tool.FlowBlocked,
			Info: "blocked by rules",
		},
		URL: "https://ads.example.com/banner",
	})
	log.Put(tool.Flow{
		FlowRow: tool.FlowRow{
			Source: tool.SourcePacket, Host: "8.8.8.8:53", Proto: "DNS",
			Size: 74, Info: "DNS query example.org",
		},
		Packet: &tool.PacketDetail{Raw: []byte{1, 2, 3}},
	})
	return log
}

func TestFilterFields(t *testing.T) {
	log := filterLog()
	tests := []struct {
		name string
		in   tool.Filter
		want []uint64
	}{
		{"zero value is the proxy source", tool.Filter{}, []uint64{1, 2, 3, 4, 5}},
		{"packets", tool.Filter{Source: tool.SourcePacket}, []uint64{6}},
		{"packet text in the note", tool.Filter{Source: tool.SourcePacket, Text: "dns"}, []uint64{6}},
		{"text in the URL, any case", tool.Filter{Text: "EXAMPLE"}, []uint64{1, 2, 4, 5}},
		{"text in a text response body", tool.Filter{Text: "not found"}, []uint64{2}},
		{"text in a text request body", tool.Filter{Text: "widget"}, []uint64{2}},
		{"text in a header value", tool.Filter{Text: "abc123"}, []uint64{1}},
		{"text in a header name", tool.Filter{Text: "x-trace"}, []uint64{1}},
		{"text in the note of a proxy flow", tool.Filter{Text: "connection reset"}, []uint64{3}},
		{"text skips a body that is not text", tool.Filter{Text: "needle in bytes"}, nil},
		{"host with or without the port", tool.Filter{Hosts: []string{"example.com"}}, []uint64{1, 4}},
		{"host glob", tool.Filter{Hosts: []string{"*.example.com"}}, []uint64{2, 5}},
		{"host and port, any case", tool.Filter{Hosts: []string{"EXAMPLE.COM:443"}}, []uint64{1, 4}},
		{"host wildcard", tool.Filter{Hosts: []string{"*"}}, []uint64{1, 2, 3, 4, 5}},
		{"methods, any case", tool.Filter{Methods: []string{"get"}}, []uint64{1, 3, 5}},
		{"method", tool.Filter{Methods: []string{"POST"}}, []uint64{2}},
		{"type prefix", tool.Filter{Types: []string{"image/"}}, []uint64{3}},
		{"type prefix of an application", tool.Filter{Types: []string{"application/"}}, []uint64{2}},
		{"type whole", tool.Filter{Types: []string{"text/html"}}, []uint64{1}},
		{"status class", tool.Filter{StatusClasses: []int{2}}, []uint64{1}},
		{"status class of no response", tool.Filter{StatusClasses: []int{0}}, []uint64{4, 5}},
		{"status classes", tool.Filter{StatusClasses: []int{4, 5}}, []uint64{2, 3}},
		{"failed", tool.Filter{Failed: true}, []uint64{3, 5}},
		{
			"every field at once",
			tool.Filter{
				Source:        tool.SourceProxy,
				Text:          "example",
				Hosts:         []string{"example.com"},
				Methods:       []string{"GET"},
				Types:         []string{"text/"},
				StatusClasses: []int{2},
			},
			[]uint64{1},
		},
		{
			"failed with text",
			tool.Filter{Source: tool.SourceProxy, Failed: true, Text: "rules"},
			[]uint64{5},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertIDs(t, log.Rows(tt.in), tt.want...)
		})
	}
}
