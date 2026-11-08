package comp

import (
	"bytes"
	"encoding/json"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// BodyKind is what a formatted body is; a view shows each kind its own
// way.
type BodyKind int

// The kinds of bodies.
const (
	// BodyEmpty is a message that carried nothing.
	BodyEmpty BodyKind = iota
	// BodyJSON is text that is JSON, shown pretty-printed.
	BodyJSON
	// BodyText is plain text, shown as it is.
	BodyText
	// BodyForm is a form body, shown as one key=value pair a line.
	BodyForm
	// BodyImage is an image, named rather than drawn.
	BodyImage
	// BodyBinary is anything else, shown as a hex dump.
	BodyBinary
)

// jsonIndent is the indentation of pretty-printed JSON.
const jsonIndent = 2

// formBody is the media type of a form a page sent.
const formBody = "application/x-www-form-urlencoded"

// FormatBody returns the text a view shows of a body, and what the body
// is. JSON is pretty-printed, with the raw text when it does not parse;
// a form is decoded to its pairs; an image is its media type alone; any
// other binary is a hex dump of its first ds.HexDumpBytes bytes. It is
// pure, so a view may call it while drawing.
func FormatBody(b tool.Body) (string, BodyKind) {
	if len(b.Data) == 0 {
		return "", BodyEmpty
	}
	media := strings.ToLower(strings.TrimSpace(b.Type))
	if isImage(media) {
		return media, BodyImage
	}
	if media == formBody {
		return formLines(b.Data), BodyForm
	}
	if isJSON(media) {
		if pretty, ok := prettyJSON(b.Data); ok {
			return pretty, BodyJSON
		}
		return string(b.Data), BodyText
	}
	if isText(media) {
		return string(b.Data), BodyText
	}
	// A type nobody declared: trust the bytes rather than the label.
	if !looksText(b.Data) {
		return HexDump(head(b.Data, ds.HexDumpBytes)), BodyBinary
	}
	if pretty, ok := prettyJSON(b.Data); ok {
		return pretty, BodyJSON
	}
	return string(b.Data), BodyText
}

// isImage reports whether a media type is an image the view names rather
// than draws. SVG is text.
func isImage(media string) bool {
	return strings.HasPrefix(media, "image/") && !strings.Contains(media, "svg")
}

// isJSON reports whether a media type carries JSON: application/json,
// text/json, and the +json suffix.
func isJSON(media string) bool {
	return strings.HasSuffix(media, "json")
}

// isText reports whether a media type is text a person reads.
func isText(media string) bool {
	switch {
	case strings.HasPrefix(media, "text/"),
		strings.Contains(media, "xml"),
		strings.Contains(media, "javascript"),
		strings.Contains(media, "ecmascript"),
		strings.Contains(media, "yaml"),
		strings.Contains(media, "csv"),
		strings.Contains(media, "graphql"),
		strings.Contains(media, "x-sh"),
		strings.Contains(media, "svg"):
		return true
	}
	return false
}

// prettyJSON indents JSON with jsonIndent spaces, keeping its key order.
// ok is false when the bytes are not an object or an array of valid JSON.
func prettyJSON(data []byte) (string, bool) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || (trimmed[0] != '{' && trimmed[0] != '[') {
		return "", false
	}
	if !json.Valid(data) {
		return "", false
	}
	var out bytes.Buffer
	if err := json.Indent(&out, data, "", strings.Repeat(" ", jsonIndent)); err != nil {
		return "", false
	}
	return out.String(), true
}

// formLines decodes a form body to one key=value pair a line, in the
// order it was sent, leaving pairs that do not decode as they are.
func formLines(data []byte) string {
	pairs := strings.Split(string(data), "&")
	lines := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		if pair == "" {
			continue
		}
		key, value, found := strings.Cut(pair, "=")
		if !found {
			lines = append(lines, decode(key))
			continue
		}
		lines = append(lines, decode(key)+"="+decode(value))
	}
	return strings.Join(lines, "\n")
}

// decode unescapes one form component, keeping it as it is when it does
// not decode.
func decode(s string) string {
	if out, err := url.QueryUnescape(s); err == nil {
		return out
	}
	return s
}

// looksText reports whether bytes read as text: valid UTF-8 without the
// control characters a text file does not hold.
func looksText(data []byte) bool {
	if !utf8.Valid(data) {
		return false
	}
	for _, c := range data {
		switch {
		case c < ' ' && c != '\t' && c != '\n' && c != '\r':
			return false
		case c == 0x7f:
			return false
		}
	}
	return true
}

// head returns the first n bytes of data, or data when it is shorter.
func head(data []byte, n int) []byte {
	if len(data) <= n {
		return data
	}
	return data[:n]
}
