package proxy

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"io"
	"mime"
	"net/http"
	"strings"
	"sync"

	"github.com/andybalholm/brotli"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// bodyCapture reads a body through and keeps at most MaxBody bytes of it for
// the flow; the client still gets every byte. An HTTP/2 transport reads and
// closes request bodies from its own goroutines, so every field is guarded.
type bodyCapture struct {
	mu        sync.Mutex
	r         io.Reader
	buf       []byte
	total     int64
	truncated bool
	done      bool
}

// newBodyCapture streams r through a capture.
func newBodyCapture(r io.Reader) *bodyCapture {
	return &bodyCapture{r: r}
}

// Read reads from the body, keeping what the flow may show.
func (c *bodyCapture) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.mu.Lock()
	if n > 0 {
		c.total += int64(n)
		room := tool.MaxBody - len(c.buf)
		switch {
		case room <= 0:
			c.truncated = true
		case n <= room:
			c.buf = append(c.buf, p[:n]...)
		default:
			c.buf = append(c.buf, p[:room]...)
			c.truncated = true
		}
	}
	if err != nil {
		c.done = true
	}
	c.mu.Unlock()
	return n, err
}

// finished reports whether the body has reached its end.
func (c *bodyCapture) finished() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.done
}

// snapshot is what the capture holds, taken under its lock.
func (c *bodyCapture) snapshot() (data []byte, total int64, truncated bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf, c.total, c.truncated
}

// body turns what was captured into the body a flow keeps, with its content
// encoding removed when that is known.
func (c *bodyCapture) body(contentType, contentEncoding string) tool.Body {
	data, total, truncated := c.snapshot()
	decoded, cut := decodeBody(data, contentEncoding)
	return tool.Body{
		Type:      mediaType(contentType),
		Data:      decoded,
		Size:      total,
		Truncated: truncated || cut,
	}
}

// streamBody is a body that is captured as it streams to the client and that
// still closes what it reads from.
type streamBody struct {
	io.Reader
	closer io.Closer
}

// Close closes the body the capture reads from.
func (b *streamBody) Close() error { return b.closer.Close() }

// captureStream wraps a body so its bytes are recorded while they pass.
func captureStream(body io.ReadCloser) (*bodyCapture, io.ReadCloser) {
	capture := newBodyCapture(body)
	return capture, &streamBody{Reader: capture, closer: body}
}

// requestBody is the body the proxy sends on. It records what passes, and
// its Close never separates the original body from the client connection: an
// HTTP/2 transport closes a request body from its own goroutine after the
// server may have answered early, and draining the client connection from
// there would race with the next request and let its bytes be read twice.
type requestBody struct {
	capture  *bodyCapture
	original io.ReadCloser
	mu       sync.Mutex
	closed   bool
}

// Read reads the body unless it has been closed.
func (b *requestBody) Read(p []byte) (int, error) {
	b.mu.Lock()
	closed := b.closed
	b.mu.Unlock()
	if closed {
		return 0, http.ErrBodyReadAfterClose
	}
	return b.capture.Read(p)
}

// Close forgets the body without touching the client connection.
func (b *requestBody) Close() error {
	b.mu.Lock()
	b.closed = true
	b.mu.Unlock()
	return nil
}

// release closes the original body, and only when every byte was read: an
// unread body belongs to a connection the proxy must drop instead.
func (b *requestBody) release() {
	if b.capture.finished() {
		_ = b.original.Close()
	}
}

// captureRequest wraps the body of a request that is sent on.
func captureRequest(req *http.Request) *requestBody {
	if req.Body == nil || req.Body == http.NoBody {
		return nil
	}
	body := &requestBody{capture: newBodyCapture(req.Body), original: req.Body}
	req.Body = body
	return body
}

// maxEncodingLayers bounds the content encodings the proxy undoes. A longer
// chain costs more than the record is worth, and is likely hostile.
const maxEncodingLayers = 4

// decoderFor returns the function that undoes one content encoding, or nil
// for an encoding the proxy cannot undo. This is the one list of encodings
// the proxy knows: decoding and replay both ask it.
func decoderFor(encoding string) func([]byte, int) ([]byte, bool, error) {
	switch encoding {
	case "gzip", "x-gzip":
		return decodeGzip
	case "deflate":
		return decodeDeflate
	case "br":
		return decodeBrotli
	}
	return nil
}

// encodingTokens splits a Content-Encoding value into its layers, in the
// order they were applied, dropping the ones that remove nothing.
func encodingTokens(encoding string) []string {
	parts := strings.Split(encoding, ",")
	tokens := make([]string, 0, len(parts))
	for _, part := range parts {
		token := strings.ToLower(strings.TrimSpace(part))
		if token == "" || token == "identity" {
			continue
		}
		tokens = append(tokens, token)
	}
	return tokens
}

// decodedByProxy reports whether decodeBody would remove the whole encodings
// of a header, so the body a flow kept is the decoded one.
func decodedByProxy(encoding string) bool {
	tokens := encodingTokens(encoding)
	if len(tokens) == 0 || len(tokens) > maxEncodingLayers {
		return false
	}
	for _, token := range tokens {
		if decoderFor(token) == nil {
			return false
		}
	}
	return true
}

// decodeBody removes the content encoding from data, in reverse order. It
// keeps the bytes unchanged when an encoding is unknown, when the chain is
// longer than maxEncodingLayers, or when the chain decodes more than the
// record keeps: a hostile chain must not cost more than a plain body. The
// bool says the decoded body was cut at MaxBody.
func decodeBody(data []byte, encoding string) ([]byte, bool) {
	raw := data
	tokens := encodingTokens(encoding)
	if len(tokens) > maxEncodingLayers {
		return raw, false
	}
	budget := tool.MaxBody
	for i := len(tokens) - 1; i >= 0; i-- {
		decoder := decoderFor(tokens[i])
		if decoder == nil {
			return raw, false
		}
		decoded, more, err := decoder(data, budget)
		if err != nil {
			return raw, false
		}
		if more {
			if i > 0 || budget == 0 {
				return raw, false
			}
			return decoded[:budget], true
		}
		data = decoded
		budget -= len(data)
	}
	if len(data) > tool.MaxBody {
		return data[:tool.MaxBody], true
	}
	return data, false
}

// decodeGzip undoes gzip, keeping at most limit bytes.
func decodeGzip(data []byte, limit int) ([]byte, bool, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, false, err
	}
	return readCapped(reader, limit)
}

// decodeDeflate undoes zlib-wrapped deflate, or raw deflate, keeping at most
// limit bytes.
func decodeDeflate(data []byte, limit int) ([]byte, bool, error) {
	if reader, err := zlib.NewReader(bytes.NewReader(data)); err == nil {
		return readCapped(reader, limit)
	}
	return readCapped(flate.NewReader(bytes.NewReader(data)), limit)
}

// decodeBrotli undoes brotli, keeping at most limit bytes.
func decodeBrotli(data []byte, limit int) ([]byte, bool, error) {
	return readCapped(brotli.NewReader(bytes.NewReader(data)), limit)
}

// readCapped reads a decoder, asking for one byte past limit so a body that
// grows without end is noticed and cut instead of filling memory.
func readCapped(r io.Reader, limit int) ([]byte, bool, error) {
	defer func() {
		if closer, ok := r.(io.Closer); ok {
			_ = closer.Close()
		}
	}()
	out, err := io.ReadAll(io.LimitReader(r, int64(limit)+1))
	if err != nil {
		return nil, false, err
	}
	if len(out) > limit {
		return out, true, nil
	}
	return out, false, nil
}

// mediaType is the media type of a Content-Type header without parameters.
func mediaType(contentType string) string {
	if contentType == "" {
		return ""
	}
	parsed, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return ""
	}
	return parsed
}
