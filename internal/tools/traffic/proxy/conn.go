package proxy

import (
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// teeWindow is how much of the last bytes read from a connection are kept,
// so a message head can be recovered after net/http has parsed it.
const teeWindow = 64 << 10

// maxHeadBytes bounds one message head. A peer that never ends its header
// block would otherwise grow the app's memory until it dies.
const maxHeadBytes = 1 << 20

// errHeadTooLarge is what a read reports when a message head passes the
// limit.
var errHeadTooLarge = errors.New("message head is too large")

// headLimiter stops a message head from growing without bound. It is armed
// before net/http parses a head and lifted once the head has been read, so
// bodies keep streaming freely.
type headLimiter struct {
	r     io.Reader
	tee   *teeConn
	left  int
	armed bool
}

// begin starts a new budget and recording window for a message head.
func (l *headLimiter) begin() {
	l.left = maxHeadBytes
	l.armed = true
	if l.tee != nil {
		l.tee.begin()
	}
}

// next restarts the budget for the next interim head of the same message;
// the recording window keeps what it has.
func (l *headLimiter) next() { l.left = maxHeadBytes }

// lift lets everything through again after the last head of a message.
func (l *headLimiter) lift() {
	l.armed = false
	if l.tee != nil {
		l.tee.lift()
	}
}

// Read reads on until the budget is spent.
func (l *headLimiter) Read(p []byte) (int, error) {
	if !l.armed {
		return l.r.Read(p)
	}
	if l.left <= 0 {
		return 0, errHeadTooLarge
	}
	if len(p) > l.left {
		p = p[:l.left]
	}
	n, err := l.r.Read(p)
	l.left -= n
	return n, err
}

// crlfcrlf ends a message head.
var crlfcrlf = []byte("\r\n\r\n")

// hopByHopNames are the headers a proxy must not pass on (RFC 9110 7.6.1),
// as a pipe separated constant so they are not a package variable.
const hopByHopNames = "|connection|proxy-connection|keep-alive|proxy-authenticate|proxy-authorization|te|trailer|transfer-encoding|upgrade|"

// teeConn keeps the bytes of a message head, so the proxy can record the
// headers exactly as they were sent, in order and with their case. It
// records only while a head is being read: a blind tunnel carries megabytes
// of frames, and copying them would cost more than the connection is worth.
type teeConn struct {
	net.Conn

	mu    sync.Mutex
	head  []byte
	first int64
	total int64
	armed bool
}

// begin starts a new recording window at the current position.
func (t *teeConn) begin() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.head = t.head[:0]
	t.first = t.total
	t.armed = true
}

// lift stops recording; what was recorded stays for headFrom.
func (t *teeConn) lift() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.armed = false
}

// Read reads from the connection and keeps the bytes while a head records.
func (t *teeConn) Read(p []byte) (int, error) {
	n, err := t.Conn.Read(p)
	if n > 0 {
		t.mu.Lock()
		t.total += int64(n)
		if t.armed {
			t.head = append(t.head, p[:n]...)
			if len(t.head) > teeWindow {
				drop := len(t.head) - teeWindow
				t.head = append(t.head[:0], t.head[drop:]...)
				t.first += int64(drop)
			}
		}
		t.mu.Unlock()
	}
	return n, err
}

// mark is the number of bytes read so far; headFrom starts there.
func (t *teeConn) mark() int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.total
}

// headFrom returns the message head that starts at mark, or nil when it has
// not fully arrived or has been trimmed out of the window.
func (t *teeConn) headFrom(mark int64) []byte {
	t.mu.Lock()
	defer t.mu.Unlock()
	offset := mark - t.first
	if offset < 0 || offset > int64(len(t.head)) {
		return nil
	}
	rest := t.head[offset:]
	if end := bytes.Index(rest, crlfcrlf); end >= 0 {
		return append([]byte(nil), rest[:end+len(crlfcrlf)]...)
	}
	return nil
}

// parseHead splits a raw message head into its header lines, in the order
// and with the case they arrived in.
func parseHead(raw []byte) []tool.Header {
	if len(raw) == 0 {
		return nil
	}
	lines := strings.Split(string(raw), "\r\n")
	headers := make([]tool.Header, 0, len(lines)-1)
	for _, line := range lines[1:] {
		if line == "" {
			break
		}
		if line[0] == ' ' || line[0] == '\t' {
			if len(headers) > 0 {
				headers[len(headers)-1].Value += " " + strings.TrimSpace(line)
			}
			continue
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		headers = append(headers, tool.Header{Name: name, Value: strings.TrimSpace(value)})
	}
	return headers
}

// bufferedConn reads through a reader that already holds bytes of the
// connection and writes to the connection itself.
type bufferedConn struct {
	net.Conn
	r io.Reader
}

// Read reads from the buffered reader first, then from the connection.
func (c *bufferedConn) Read(p []byte) (int, error) { return c.r.Read(p) }

// hopByHop reports whether a header is one a proxy must not forward.
func hopByHop(name string) bool {
	return strings.Contains(hopByHopNames, "|"+strings.ToLower(name)+"|")
}

// stripHopByHop removes the hop-by-hop headers from h, including those the
// Connection header names.
func stripHopByHop(h http.Header) {
	for _, value := range h.Values("Connection") {
		for _, name := range strings.Split(value, ",") {
			h.Del(strings.TrimSpace(name))
		}
	}
	for _, name := range strings.Split(hopByHopNames, "|") {
		if name != "" {
			h.Del(name)
		}
	}
}

// headerList turns a header map into the ordered list a flow keeps, sorted
// by name: a map has lost the order the headers arrived in.
func headerList(h http.Header) []tool.Header {
	names := make([]string, 0, len(h))
	for name := range h {
		names = append(names, name)
	}
	sort.Strings(names)
	headers := make([]tool.Header, 0, len(names))
	for _, name := range names {
		for _, value := range h[name] {
			headers = append(headers, tool.Header{Name: name, Value: value})
		}
	}
	return headers
}

// isUpgrade reports whether the request asks to switch protocols.
func isUpgrade(req *http.Request) bool {
	if req.Header.Get("Upgrade") == "" {
		return false
	}
	for _, value := range req.Header.Values("Connection") {
		for _, token := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(token), "upgrade") {
				return true
			}
		}
	}
	return false
}
