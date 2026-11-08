package comp

import (
	"bytes"
	"strings"
	"testing"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

func TestHexDump(t *testing.T) {
	bytes0to15 := make([]byte, 16)
	for i := range bytes0to15 {
		bytes0to15[i] = byte(i)
	}
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{"nothing", nil, ""},
		{
			name: "a full row",
			data: []byte("HTTP/1.1 200 OK\n"),
			want: "00000000  48 54 54 50 2f 31 2e 31  20 32 30 30 20 4f 4b 0a  |HTTP/1.1 200 OK.|",
		},
		{
			name: "bytes that are not printable are dots",
			data: bytes0to15,
			want: "00000000  00 01 02 03 04 05 06 07  08 09 0a 0b 0c 0d 0e 0f  |................|",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HexDump(tt.data); got != tt.want {
				t.Errorf("HexDump() =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestHexDumpRows(t *testing.T) {
	got := HexDump([]byte("abcdefghijklmnopqrst"))
	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Fatalf("%d rows:\n%s", len(lines), got)
	}
	if !strings.HasPrefix(lines[1], "00000010  ") {
		t.Errorf("second row starts %q, want its address", lines[1])
	}
	if !strings.HasSuffix(lines[1], "|qrst|") {
		t.Errorf("second row ends %q, want the rest of the bytes", lines[1])
	}
	if n := strings.Count(lines[1], "   "); n == 0 {
		t.Errorf("a short row is not padded:\n%s", lines[1])
	}
}

func TestHexDumpASCII(t *testing.T) {
	data := []byte{' ', '~', 0x1f, 0x7f, 'A'}
	got := HexDump(data)
	if !strings.HasSuffix(got, "| ~..A|") {
		t.Errorf("HexDump() = %q, want its characters as themselves, 0x1f and 0x7f as dots", got)
	}
}

func TestFormatBody(t *testing.T) {
	tests := []struct {
		name string
		body tool.Body
		want string
		kind BodyKind
	}{
		{"nothing", tool.Body{}, "", BodyEmpty},
		{
			name: "json is pretty-printed",
			body: tool.Body{Type: "application/json", Data: []byte(`{"a":1,"b":[2,3]}`)},
			want: "{\n  \"a\": 1,\n  \"b\": [\n    2,\n    3\n  ]\n}",
			kind: BodyJSON,
		},
		{
			name: "json that does not parse is text",
			body: tool.Body{Type: "application/json", Data: []byte(`{oops`)},
			want: `{oops`,
			kind: BodyText,
		},
		{
			name: "html is text as it is",
			body: tool.Body{Type: "text/html", Data: []byte("<h1>hi</h1>")},
			want: "<h1>hi</h1>",
			kind: BodyText,
		},
		{
			name: "a form is decoded to its pairs",
			body: tool.Body{Type: "application/x-www-form-urlencoded", Data: []byte("user=ada%20l&remember=1&flag")},
			want: "user=ada l\nremember=1\nflag",
			kind: BodyForm,
		},
		{
			name: "an image is named",
			body: tool.Body{Type: "image/png", Data: []byte("\x89PNG")},
			want: "image/png",
			kind: BodyImage,
		},
		{
			name: "svg is text",
			body: tool.Body{Type: "image/svg+xml", Data: []byte("<svg/>")},
			want: "<svg/>",
			kind: BodyText,
		},
		{
			name: "an unknown type trusts text bytes",
			body: tool.Body{Type: "application/x-thing", Data: []byte("hello")},
			want: "hello",
			kind: BodyText,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, kind := FormatBody(tt.body)
			if got != tt.want || kind != tt.kind {
				t.Errorf("FormatBody() = (%q, %v), want (%q, %v)", got, kind, tt.want, tt.kind)
			}
		})
	}
}

func TestFormatBodyBinary(t *testing.T) {
	data := []byte{0x00, 0xff, 0x10}
	got, kind := FormatBody(tool.Body{Type: "application/octet-stream", Data: data})
	if kind != BodyBinary {
		t.Fatalf("kind %v, want a binary body", kind)
	}
	if want := HexDump(data); got != want {
		t.Errorf("FormatBody() = %q, want the hex dump %q", got, want)
	}
}

func TestFormatBodyBinaryIsCutForTheDump(t *testing.T) {
	data := bytes.Repeat([]byte{0x00}, 2*ds.HexDumpBytes)
	got, kind := FormatBody(tool.Body{Type: "application/octet-stream", Data: data})
	if kind != BodyBinary {
		t.Fatalf("kind %v, want a binary body", kind)
	}
	rows := strings.Count(got, "\n") + 1
	if want := ds.HexDumpBytes / ds.HexRow; rows != want {
		t.Errorf("the dump has %d rows, want %d: only its head is shown", rows, want)
	}
}

func TestCutText(t *testing.T) {
	if got, cut := cutText("short", 10); got != "short" || cut {
		t.Errorf("cutText(short) = (%q, %v), want it whole", got, cut)
	}
	whole := "héllo"
	got, cut := cutText(whole, 2) // the second byte is inside é
	if !cut || !strings.Contains(whole, got) {
		t.Errorf("cutText split a character: %q, cut %v", got, cut)
	}
	if len(got) > 2 {
		t.Errorf("cutText kept %d bytes, want at most 2", len(got))
	}
}
