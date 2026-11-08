package rules

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// response builds a 200 response a test hands to the engine. It is a value
// so that the engine, not the test, owns the body.
func response(header http.Header, body string) http.Response {
	data := []byte(body)
	return http.Response{
		Status:        statusLine(http.StatusOK),
		StatusCode:    http.StatusOK,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        header,
		Body:          io.NopCloser(bytes.NewReader(data)),
		ContentLength: int64(len(data)),
	}
}

// gzipped is body compressed with gzip.
func gzipped(t *testing.T, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write([]byte(body)); err != nil {
		t.Fatalf("gzip body: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}
	return buf.Bytes()
}

// deflated is body compressed with zlib.
func deflated(t *testing.T, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	if _, err := zw.Write([]byte(body)); err != nil {
		t.Fatalf("deflate body: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zlib writer: %v", err)
	}
	return buf.Bytes()
}

func TestResponseRewriteChain(t *testing.T) {
	e := New()
	mustSet(t, e,
		rewriteRule("headers", tool.RewriteEffect{ResponseHeaders: []tool.HeaderEdit{
			{Name: "x-added", Value: "1"},
			{Name: "X-Drop", Remove: true},
		}}),
		rewriteRule("body", tool.RewriteEffect{ResponseBody: &tool.BodyEdit{Find: "secret", Replace: "public"}}),
		rewriteRule("status", tool.RewriteEffect{Status: http.StatusTeapot}),
	)
	req := newRequest(t, http.MethodGet, "https://example.com/x", nil)
	resp := response(http.Header{
		"Content-Type": {"text/plain"},
		"X-Drop":       {"yes"},
	}, "hello secret world")

	hits, err := e.Response(req, &resp)
	if err != nil {
		t.Fatalf("response: %v", err)
	}
	if !slices.Equal(hits, []string{"headers", "body", "status"}) {
		t.Errorf("hits %v, want the three rewrites in order", hits)
	}
	if got := readAll(t, resp.Body); got != "hello public world" {
		t.Errorf("body is %q", got)
	}
	if resp.ContentLength != 18 || resp.Header.Get("Content-Length") != "18" {
		t.Errorf("length is %d, header %q", resp.ContentLength, resp.Header.Get("Content-Length"))
	}
	if resp.Header.Get("X-Added") != "1" {
		t.Errorf("X-Added is %q", resp.Header.Get("X-Added"))
	}
	if _, held := resp.Header["X-Drop"]; held {
		t.Error("X-Drop was not removed")
	}
	if resp.StatusCode != http.StatusTeapot || resp.Status != "418 I'm a teapot" {
		t.Errorf("status is %d %q", resp.StatusCode, resp.Status)
	}
}

func TestResponseGzipBodyIsDecodedAndPlain(t *testing.T) {
	e := New()
	mustSet(t, e, rewriteRule("gz", tool.RewriteEffect{
		ResponseBody: &tool.BodyEdit{Find: "secret", Replace: "public"},
	}))
	raw := gzipped(t, "hello secret")
	resp := &http.Response{
		Status: "200 OK", StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Encoding": {"gzip"},
			"Content-Length":   {strconv.Itoa(len(raw))},
		},
		Body:          io.NopCloser(bytes.NewReader(raw)),
		ContentLength: int64(len(raw)),
	}
	hits, err := e.Response(newRequest(t, http.MethodGet, "https://example.com/x", nil), resp)
	if err != nil {
		t.Fatalf("response: %v", err)
	}
	if !slices.Equal(hits, []string{"gz"}) {
		t.Errorf("hits %v, want the gzip rewrite", hits)
	}
	if got := readAll(t, resp.Body); got != "hello public" {
		t.Errorf("body is %q, want plain bytes", got)
	}
	if got := resp.Header.Get("Content-Encoding"); got != "" {
		t.Errorf("Content-Encoding is still %q", got)
	}
	if resp.ContentLength != 12 || resp.Header.Get("Content-Length") != "12" {
		t.Errorf("length is %d, header %q", resp.ContentLength, resp.Header.Get("Content-Length"))
	}
}

func TestResponseDeflateBodyIsDecodedAndPlain(t *testing.T) {
	e := New()
	mustSet(t, e, rewriteRule("df", tool.RewriteEffect{
		ResponseBody: &tool.BodyEdit{Find: "secret", Replace: "public"},
	}))
	raw := deflated(t, "hello secret")
	resp := &http.Response{
		Status: "200 OK", StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Encoding": {"deflate"},
			"Content-Length":   {strconv.Itoa(len(raw))},
		},
		Body:          io.NopCloser(bytes.NewReader(raw)),
		ContentLength: int64(len(raw)),
	}
	if _, err := e.Response(newRequest(t, http.MethodGet, "https://example.com/x", nil), resp); err != nil {
		t.Fatalf("response: %v", err)
	}
	if got := readAll(t, resp.Body); got != "hello public" {
		t.Fatalf("body is %q, want plain bytes", got)
	}
	if got := resp.Header.Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding is still %q", got)
	}
}

func TestResponseKeepsAnEncodingItCannotDecode(t *testing.T) {
	e := New()
	mustSet(t, e, rewriteRule("zstd", tool.RewriteEffect{
		ResponseBody: &tool.BodyEdit{Find: "secret", Replace: "public"},
	}))
	resp := response(http.Header{
		"Content-Encoding": {"zstd"},
		"Content-Length":   {"12"},
	}, "hello secret")
	if _, err := e.Response(newRequest(t, http.MethodGet, "https://example.com/x", nil), &resp); err != nil {
		t.Fatalf("response: %v", err)
	}
	if got := readAll(t, resp.Body); got != "hello public" {
		t.Fatalf("body is %q", got)
	}
	if got := resp.Header.Get("Content-Encoding"); got != "zstd" {
		t.Fatalf("Content-Encoding is %q, want zstd kept", got)
	}
}

func TestResponseIgnoresRulesOfOtherKinds(t *testing.T) {
	e := New()
	mustSet(t, e, mockRule("m", tool.MockEffect{Body: "never"}), blockRule("b"), delayRule("d", 1))
	resp := response(http.Header{"X-Keep": {"1"}}, "body")
	hits, err := e.Response(newRequest(t, http.MethodGet, "https://example.com/x", nil), &resp)
	if err != nil {
		t.Fatalf("response: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("hits %v, want none", hits)
	}
	if got := readAll(t, resp.Body); got != "body" {
		t.Fatalf("body is %q, want it untouched", got)
	}
}

func TestResponseBodyOverTheLimitIsLeftAlone(t *testing.T) {
	e := New()
	mustSet(t, e, rewriteRule("big", tool.RewriteEffect{
		ResponseHeaders: []tool.HeaderEdit{{Name: "X-Skipped", Value: "1"}},
		ResponseBody:    &tool.BodyEdit{Replace: "small"},
	}))
	body := strings.Repeat("a", tool.MaxBody+1)
	resp := response(http.Header{}, body)
	hits, err := e.Response(newRequest(t, http.MethodGet, "https://example.com/x", nil), &resp)
	if !errors.Is(err, ErrBodyTooLarge) {
		t.Fatalf("error is %v, want ErrBodyTooLarge", err)
	}
	if len(hits) != 0 {
		t.Fatalf("hits %v, want none", hits)
	}
	if got := resp.Header.Get("X-Skipped"); got != "" {
		t.Error("the skipped rule still edited the response")
	}
	if got := readAll(t, resp.Body); got != body {
		t.Fatalf("the body lost bytes: %d of %d", len(got), len(body))
	}
}

func TestResponseWithoutBodyEditLeavesTheBody(t *testing.T) {
	e := New()
	mustSet(t, e, rewriteRule("status", tool.RewriteEffect{Status: http.StatusAccepted}))
	resp := response(http.Header{"Content-Encoding": {"gzip"}}, "hello")
	if _, err := e.Response(newRequest(t, http.MethodGet, "https://example.com/x", nil), &resp); err != nil {
		t.Fatalf("response: %v", err)
	}
	if got := readAll(t, resp.Body); got != "hello" {
		t.Fatalf("body is %q, want it untouched", got)
	}
	if got := resp.Header.Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding is %q, want it kept", got)
	}
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status is %d", resp.StatusCode)
	}
}

func TestResponseBadGzipIsReportedAndLeftAlone(t *testing.T) {
	e := New()
	mustSet(t, e, rewriteRule("gz", tool.RewriteEffect{
		ResponseBody: &tool.BodyEdit{Find: "a", Replace: "b"},
	}))
	resp := &http.Response{
		Status: "200 OK", StatusCode: http.StatusOK,
		Header:        http.Header{"Content-Encoding": {"gzip"}},
		Body:          io.NopCloser(strings.NewReader("not gzip at all")),
		ContentLength: 15,
	}
	hits, err := e.Response(newRequest(t, http.MethodGet, "https://example.com/x", nil), resp)
	if err == nil {
		t.Fatal("a broken gzip body went unnoticed")
	}
	if !strings.Contains(err.Error(), "decode gzip body") {
		t.Fatalf("error %q does not say what failed", err)
	}
	if len(hits) != 0 {
		t.Fatalf("hits %v, want none", hits)
	}
	if got := readAll(t, resp.Body); got != "not gzip at all" {
		t.Fatalf("body is %q, want it restored", got)
	}
	if got := resp.Header.Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding is %q, want it kept", got)
	}
}

func TestResponseRawDeflateBody(t *testing.T) {
	var buf bytes.Buffer
	fw, err := flate.NewWriter(&buf, flate.DefaultCompression)
	if err != nil {
		t.Fatalf("flate writer: %v", err)
	}
	if _, err := fw.Write([]byte("hello secret")); err != nil {
		t.Fatalf("write deflate body: %v", err)
	}
	if err := fw.Close(); err != nil {
		t.Fatalf("close deflate writer: %v", err)
	}
	e := New()
	mustSet(t, e, rewriteRule("df", tool.RewriteEffect{
		ResponseBody: &tool.BodyEdit{Find: "secret", Replace: "public"},
	}))
	resp := &http.Response{
		Status: "200 OK", StatusCode: http.StatusOK,
		Header:        http.Header{"Content-Encoding": {"deflate"}},
		Body:          io.NopCloser(bytes.NewReader(buf.Bytes())),
		ContentLength: int64(buf.Len()),
	}
	if _, err := e.Response(newRequest(t, http.MethodGet, "https://example.com/x", nil), resp); err != nil {
		t.Fatalf("response: %v", err)
	}
	if got := readAll(t, resp.Body); got != "hello public" {
		t.Fatalf("body is %q, want plain bytes", got)
	}
	if got := resp.Header.Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding is still %q", got)
	}
}

func TestResponseDecodedBodyOverTheLimit(t *testing.T) {
	raw := gzipped(t, strings.Repeat("a", tool.MaxBody+1))
	e := New()
	mustSet(t, e, rewriteRule("gz", tool.RewriteEffect{
		ResponseBody: &tool.BodyEdit{Replace: "small"},
	}))
	resp := &http.Response{
		Status: "200 OK", StatusCode: http.StatusOK,
		Header:        http.Header{"Content-Encoding": {"gzip"}},
		Body:          io.NopCloser(bytes.NewReader(raw)),
		ContentLength: int64(len(raw)),
	}
	hits, err := e.Response(newRequest(t, http.MethodGet, "https://example.com/x", nil), resp)
	if !errors.Is(err, ErrBodyTooLarge) {
		t.Fatalf("error is %v, want ErrBodyTooLarge", err)
	}
	if len(hits) != 0 {
		t.Fatalf("hits %v, want none", hits)
	}
	got := []byte(readAll(t, resp.Body))
	if !bytes.Equal(got, raw) {
		t.Fatalf("the body lost bytes: %d of %d", len(got), len(raw))
	}
	if got := resp.Header.Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding is %q, want it kept", got)
	}
}

func TestResponseWithNoHeaderMap(t *testing.T) {
	e := New()
	mustSet(t, e, rewriteRule("rw", tool.RewriteEffect{
		ResponseBody: &tool.BodyEdit{Find: "secret", Replace: "public"},
		Status:       http.StatusAccepted,
	}))
	resp := http.Response{
		Status: "200 OK", StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader("hello secret")),
	}
	if _, err := e.Response(newRequest(t, http.MethodGet, "https://example.com/x", nil), &resp); err != nil {
		t.Fatalf("response: %v", err)
	}
	if got := readAll(t, resp.Body); got != "hello public" {
		t.Fatalf("body is %q", got)
	}
	if got := resp.Header.Get("Content-Length"); got != "12" {
		t.Fatalf("Content-Length is %q, want 12", got)
	}
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status is %d", resp.StatusCode)
	}
}

func TestResponseTruncatedGzipIsReported(t *testing.T) {
	raw := gzipped(t, strings.Repeat("hello secret ", 100))
	cut := raw[:len(raw)-8]
	e := New()
	mustSet(t, e, rewriteRule("gz", tool.RewriteEffect{
		ResponseBody: &tool.BodyEdit{Find: "secret", Replace: "public"},
	}))
	resp := &http.Response{
		Status: "200 OK", StatusCode: http.StatusOK,
		Header:        http.Header{"Content-Encoding": {"gzip"}},
		Body:          io.NopCloser(bytes.NewReader(cut)),
		ContentLength: int64(len(cut)),
	}
	hits, err := e.Response(newRequest(t, http.MethodGet, "https://example.com/x", nil), resp)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("error is %v, want the truncated stream", err)
	}
	if len(hits) != 0 {
		t.Fatalf("hits %v, want none", hits)
	}
	if got := []byte(readAll(t, resp.Body)); !bytes.Equal(got, cut) {
		t.Fatalf("the body lost bytes: %d of %d", len(got), len(cut))
	}
}

// brotlied is body compressed with brotli.
func brotlied(t *testing.T, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	bw := brotli.NewWriter(&buf)
	if _, err := bw.Write([]byte(body)); err != nil {
		t.Fatalf("brotli body: %v", err)
	}
	if err := bw.Close(); err != nil {
		t.Fatalf("close brotli writer: %v", err)
	}
	return buf.Bytes()
}

func TestResponseBrotliBodyIsDecodedAndPlain(t *testing.T) {
	raw := brotlied(t, "hello secret")
	e := New()
	mustSet(t, e, rewriteRule("br", tool.RewriteEffect{
		ResponseBody: &tool.BodyEdit{Find: "secret", Replace: "public"},
	}))
	resp := &http.Response{
		Status: "200 OK", StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Encoding": {"br"},
			"Content-Length":   {strconv.Itoa(len(raw))},
		},
		Body:          io.NopCloser(bytes.NewReader(raw)),
		ContentLength: int64(len(raw)),
	}
	hits, err := e.Response(newRequest(t, http.MethodGet, "https://example.com/x", nil), resp)
	if err != nil {
		t.Fatalf("response: %v", err)
	}
	if !slices.Equal(hits, []string{"br"}) {
		t.Errorf("hits %v, want the brotli rewrite", hits)
	}
	if got := readAll(t, resp.Body); got != "hello public" {
		t.Errorf("body is %q, want plain bytes", got)
	}
	if got := resp.Header.Get("Content-Encoding"); got != "" {
		t.Errorf("Content-Encoding is still %q", got)
	}
	if resp.ContentLength != 12 || resp.Header.Get("Content-Length") != "12" {
		t.Errorf("length is %d, header %q", resp.ContentLength, resp.Header.Get("Content-Length"))
	}
}

func TestResponseBrotliBombStopsAtTheCap(t *testing.T) {
	raw := brotlied(t, strings.Repeat("a", tool.MaxBody+1))
	e := New()
	mustSet(t, e, rewriteRule("br", tool.RewriteEffect{
		ResponseBody: &tool.BodyEdit{Replace: "small"},
	}))
	resp := &http.Response{
		Status: "200 OK", StatusCode: http.StatusOK,
		Header:        http.Header{"Content-Encoding": {"br"}},
		Body:          io.NopCloser(bytes.NewReader(raw)),
		ContentLength: int64(len(raw)),
	}
	hits, err := e.Response(newRequest(t, http.MethodGet, "https://example.com/x", nil), resp)
	if !errors.Is(err, ErrBodyTooLarge) {
		t.Fatalf("error is %v, want ErrBodyTooLarge", err)
	}
	if len(hits) != 0 {
		t.Fatalf("hits %v, want none", hits)
	}
	if got := []byte(readAll(t, resp.Body)); !bytes.Equal(got, raw) {
		t.Fatalf("the body lost bytes: %d of %d", len(got), len(raw))
	}
	if got := resp.Header.Get("Content-Encoding"); got != "br" {
		t.Fatalf("Content-Encoding is %q, want it kept", got)
	}
}
