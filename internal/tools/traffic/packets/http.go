package packets

import (
	"bytes"
	"strconv"
	"strings"
)

// HTTP/1 seen in one TCP segment: the request or status line and the
// header block that follows it, as text.

const (
	layerHTTP = "Hypertext Transfer Protocol"

	httpVersionPrefix = "HTTP/1."
)

// http decodes the head of a plain HTTP/1 message that starts a TCP
// payload, and leaves anything else alone.
func (d *decoder) http(payload []byte) {
	line, rest, ended := splitLine(payload)
	if !ended {
		if !httpStartPrefix(payload) {
			return
		}
		d.proto = protoHTTP
		d.truncated("HTTP start line")
		return
	}
	f := &fieldSet{}
	info, ok := d.httpStartLine(line, f)
	if !ok {
		return
	}
	d.proto = protoHTTP
	d.httpHeaders(rest, f)
	d.layer(layerHTTP, f)
	d.info = info
}

// httpStartLine reads the request or status line and keeps what the row
// shows of it. It reports false for a line that is neither.
func (d *decoder) httpStartLine(line []byte, f *fieldSet) (string, bool) {
	if isRequestLine(line) {
		text := printable(line)
		f.add("Request line", text)
		if parts := strings.Split(text, " "); len(parts) >= 2 {
			d.method, d.path = parts[0], parts[1]
		}
		return text, true
	}
	if bytes.HasPrefix(line, []byte(httpVersionPrefix)) {
		text := printable(line)
		f.add("Status line", text)
		if parts := strings.Split(text, " "); len(parts) >= 2 {
			if code, err := strconv.Atoi(parts[1]); err == nil {
				d.status = code
			}
		}
		return text, true
	}
	return "", false
}

// httpHeaders adds the header lines up to the blank line that ends them,
// and stops where the segment or the text ends.
func (d *decoder) httpHeaders(rest []byte, f *fieldSet) {
	for len(rest) > 0 {
		line, next, ended := splitLine(rest)
		if !ended {
			return
		}
		rest = next
		if len(line) == 0 {
			return
		}
		name, value, found := bytes.Cut(line, []byte(":"))
		if !found || !isText(line) {
			return
		}
		f.add(strings.TrimSpace(string(name)), strings.TrimSpace(string(value)))
	}
}

// splitLine cuts the first line of b without its ending, and reports
// whether b held one.
func splitLine(b []byte) (line, rest []byte, ended bool) {
	i := bytes.IndexByte(b, '\n')
	if i < 0 {
		return nil, nil, false
	}
	line = b[:i]
	if n := len(line); n > 0 && line[n-1] == '\r' {
		line = line[:n-1]
	}
	return line, b[i+1:], true
}

// isRequestLine reports whether a line starts with an HTTP method followed
// by a space.
func isRequestLine(line []byte) bool {
	for _, method := range [...]string{
		"GET", "POST", "PUT", "DELETE", "HEAD", "OPTIONS", "PATCH", "CONNECT",
	} {
		if len(line) > len(method) && line[len(method)] == ' ' && string(line[:len(method)]) == method {
			return true
		}
	}
	return false
}

// httpStartPrefix reports whether bytes cut before the first line ends
// still look like the start of an HTTP message.
func httpStartPrefix(b []byte) bool {
	if bytes.HasPrefix(b, []byte(httpVersionPrefix)) {
		return true
	}
	for _, method := range [...]string{
		"GET", "POST", "PUT", "DELETE", "HEAD", "OPTIONS", "PATCH", "CONNECT",
	} {
		if len(b) <= len(method) || string(b[:len(method)]) != method {
			continue
		}
		if b[len(method)] == ' ' {
			return true
		}
	}
	return false
}

// printable is a line as text, with control bytes and broken UTF-8
// replaced so the field never holds captured binary data.
func printable(line []byte) string {
	if isText(line) {
		return string(line)
	}
	clean := make([]byte, len(line))
	for i, c := range line {
		if c < ' ' && c != '\t' {
			c = '?'
		}
		clean[i] = c
	}
	return strings.ToValidUTF8(string(clean), "?")
}
