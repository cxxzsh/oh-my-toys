package shell

import (
	"testing"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// TestCurlCommand is the cURL text of flows, case by case, so that each rule
// of the command (method, headers, body) is pinned by an example.
func TestCurlCommand(t *testing.T) {
	const url = "https://api.example.com/v1/items"
	jsonBody := tool.Body{Type: "application/json", Data: []byte(`{"a":"it's"}`), Size: 12}
	tests := []struct {
		name string
		flow tool.Flow
		want string
	}{
		{
			name: "GET without body has no method flag",
			flow: tool.Flow{URL: url, FlowRow: tool.FlowRow{Method: "GET"}},
			want: "curl '" + url + "'",
		},
		{
			name: "empty method is GET",
			flow: tool.Flow{URL: url},
			want: "curl '" + url + "'",
		},
		{
			name: "POST with quotes in a header and a text body",
			flow: tool.Flow{
				URL:     url,
				FlowRow: tool.FlowRow{Method: "POST"},
				Request: tool.HTTPMessage{
					Headers: []tool.Header{
						{Name: "Content-Type", Value: "application/json"},
						{Name: "X-Name", Value: "it's"},
					},
					Body: jsonBody,
				},
			},
			want: `curl -X POST '` + url + `' -H 'Content-Type: application/json' -H 'X-Name: it'\''s' ` +
				`--data-raw '{"a":"it'\''s"}'`,
		},
		{
			name: "content-length and host are left to curl, pseudo-headers are not sent",
			flow: tool.Flow{
				URL:     url,
				FlowRow: tool.FlowRow{Method: "GET"},
				Request: tool.HTTPMessage{Headers: []tool.Header{
					{Name: ":authority", Value: "api.example.com"},
					{Name: ":path", Value: "/v1/items"},
					{Name: "Host", Value: "api.example.com"},
					{Name: "content-length", Value: "0"},
					{Name: "Accept-Encoding", Value: "gzip"},
				}},
			},
			want: "curl '" + url + "' -H 'Accept-Encoding: gzip'",
		},
		{
			name: "binary body is not written; the command reads it from stdin",
			flow: tool.Flow{
				URL:     url,
				FlowRow: tool.FlowRow{Method: "POST"},
				Request: tool.HTTPMessage{Body: tool.Body{Type: "application/octet-stream", Data: []byte{0xff, 0x00}}},
			},
			want: curlOmittedNote + "\ncurl -X POST '" + url + "' --data-binary @-",
		},
		{
			name: "truncated text body is omitted",
			flow: tool.Flow{
				URL:     url,
				FlowRow: tool.FlowRow{Method: "PUT"},
				Request: tool.HTTPMessage{Body: tool.Body{Type: "text/plain", Data: []byte("abc"), Truncated: true}},
			},
			want: curlOmittedNote + "\ncurl -X PUT '" + url + "' --data-binary @-",
		},
		{
			name: "text body with newlines stays inside one quoted word",
			flow: tool.Flow{
				URL:     url,
				FlowRow: tool.FlowRow{Method: "POST"},
				Request: tool.HTTPMessage{Body: tool.Body{Type: "text/plain", Data: []byte("a\nb")}},
			},
			want: "curl -X POST '" + url + "' --data-raw 'a\nb'",
		},
		{
			name: "HEAD keeps its method",
			flow: tool.Flow{URL: url, FlowRow: tool.FlowRow{Method: "HEAD"}},
			want: "curl -X HEAD '" + url + "'",
		},
		{
			name: "OPTIONS keeps its method",
			flow: tool.Flow{URL: url, FlowRow: tool.FlowRow{Method: "OPTIONS"}},
			want: "curl -X OPTIONS '" + url + "'",
		},
		{
			name: "POST with no body keeps the method",
			flow: tool.Flow{URL: url, FlowRow: tool.FlowRow{Method: "POST"}},
			want: "curl -X POST '" + url + "'",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := curlCommand(tc.flow); got != tc.want {
				t.Fatalf("curlCommand()\n got: %q\nwant: %q", got, tc.want)
			}
		})
	}
}

// TestShellQuote: a POSIX single-quoted word, with each embedded quote closed,
// escaped and reopened.
func TestShellQuote(t *testing.T) {
	tests := map[string]string{
		"":          "''",
		"plain":     "'plain'",
		"it's":      `'it'\''s'`,
		"'":         `''\'''`,
		"$HOME `x`": "'$HOME `x`'",
	}
	for in, want := range tests {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}
