package rules

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/andybalholm/brotli"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// The content encodings the engine decodes before editing a body, so the
// client is sent plain bytes rather than bytes of a broken compressed
// stream.
const (
	encodingGzip    = "gzip"
	encodingDeflate = "deflate"
	encodingBrotli  = "br"
)

// bodyEdit is a BodyEdit with its pattern compiled.
type bodyEdit struct {
	find    string
	replace string
	// whole is set when Find is empty: the whole body becomes replace.
	whole bool
	re    *regexp.Regexp
}

// compileBodyEdit compiles a body edit, if the rule has one.
func compileBodyEdit(i int, r tool.Rule, be *tool.BodyEdit) (*bodyEdit, error) {
	if be == nil {
		return nil, nil
	}
	c := &bodyEdit{find: be.Find, replace: be.Replace, whole: be.Find == ""}
	if !be.Regexp || c.whole {
		return c, nil
	}
	re, err := regexp.Compile(be.Find)
	if err != nil {
		return nil, invalidRule(i, r, "body pattern does not compile: "+err.Error())
	}
	c.re = re
	return c, nil
}

// apply returns body with the edit applied.
func (b bodyEdit) apply(body []byte) []byte {
	switch {
	case b.whole:
		return []byte(b.replace)
	case b.re != nil:
		return b.re.ReplaceAll(body, []byte(b.replace))
	default:
		return bytes.ReplaceAll(body, []byte(b.find), []byte(b.replace))
	}
}

// readBody reads a body of at most tool.MaxBody bytes. A body it cannot
// take whole is put back, so the exchange carries on with it unchanged.
func readBody(body *io.ReadCloser) ([]byte, error) {
	if *body == nil || *body == http.NoBody {
		return nil, nil
	}
	data, err := io.ReadAll(io.LimitReader(*body, tool.MaxBody+1))
	if err != nil {
		*body = rejoin(data, *body)
		return nil, fmt.Errorf("read body: %w", err)
	}
	if len(data) > tool.MaxBody {
		*body = rejoin(data, *body)
		return nil, fmt.Errorf("%w: over %d bytes", ErrBodyTooLarge, tool.MaxBody)
	}
	return data, nil
}

// rejoin makes a body of the bytes already read followed by the rest of the
// original, so a rule that gives up loses nothing.
func rejoin(read []byte, rest io.ReadCloser) io.ReadCloser {
	return &joined{Reader: io.MultiReader(bytes.NewReader(read), rest), closer: rest}
}

// joined is a re-joined body that still closes the one it came from.
type joined struct {
	io.Reader
	closer io.Closer
}

// Close closes the body the bytes came from.
func (j *joined) Close() error { return j.closer.Close() }

// setRequestBody installs the edited request body, with the lengths and
// the framing agreeing with it.
func setRequestBody(req *http.Request, data []byte) {
	closeBody(req.Body)
	req.Body = io.NopCloser(bytes.NewReader(data))
	req.ContentLength = int64(len(data))
	req.Header.Set("Content-Length", strconv.Itoa(len(data)))
	// A chunked body, or a GetBody built for the old bytes, would send them
	// again on a resend and contradict the length; both go with the body.
	req.TransferEncoding = nil
	req.GetBody = nil
}

// editResponseBody replaces a response body with the edited bytes, whose
// lengths and encoding then have to match the new bytes.
func editResponseBody(resp *http.Response, be *bodyEdit) error {
	raw, err := readBody(&resp.Body)
	if err != nil {
		return err
	}
	plain, encoded, err := decodeBody(resp.Header.Get("Content-Encoding"), raw)
	if err != nil {
		resp.Body = io.NopCloser(bytes.NewReader(raw))
		return err
	}
	edited := be.apply(plain)
	closeBody(resp.Body)
	resp.Body = io.NopCloser(bytes.NewReader(edited))
	resp.ContentLength = int64(len(edited))
	resp.Header.Set("Content-Length", strconv.Itoa(len(edited)))
	// Chunked framing contradicts a Content-Length, and a gzip header over
	// plain bytes would make the client fail to read the body.
	resp.TransferEncoding = nil
	if encoded {
		resp.Header.Del("Content-Encoding")
	}
	return nil
}

// decodeBody returns the plain bytes of a body with the given content
// encoding, and whether it had to decode it.
func decodeBody(encoding string, raw []byte) ([]byte, bool, error) {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case encodingGzip:
		zr, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, false, fmt.Errorf("decode gzip body: %w", err)
		}
		return readDecoded(zr)
	case encodingDeflate:
		return readDecoded(inflate(raw))
	case encodingBrotli:
		return readDecoded(io.NopCloser(brotli.NewReader(bytes.NewReader(raw))))
	default:
		return raw, false, nil
	}
}

// readDecoded reads a decoded body, capped like any other.
func readDecoded(rc io.ReadCloser) ([]byte, bool, error) {
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(io.LimitReader(rc, tool.MaxBody+1))
	if err != nil {
		return nil, false, fmt.Errorf("decode body: %w", err)
	}
	if len(data) > tool.MaxBody {
		return nil, false, fmt.Errorf("%w: over %d bytes decoded", ErrBodyTooLarge, tool.MaxBody)
	}
	return data, true, nil
}

// inflate opens a "deflate" body, which HTTP defines as zlib-wrapped but
// many servers send raw; both are accepted.
func inflate(raw []byte) io.ReadCloser {
	if zr, err := zlib.NewReader(bytes.NewReader(raw)); err == nil {
		return zr
	}
	return flate.NewReader(bytes.NewReader(raw))
}

// closeBody closes a body, ignoring a close error that has nowhere to go.
func closeBody(body io.Closer) {
	if body == nil || body == http.NoBody {
		return
	}
	_ = body.Close()
}
