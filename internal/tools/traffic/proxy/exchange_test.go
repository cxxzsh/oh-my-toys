package proxy

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/andybalholm/brotli"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

func TestProxyKeepsTheClientConnection(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "kept")
	}))
	t.Cleanup(upstream.Close)
	recorded := newTestRecorder()
	srv := startProxy(t, Options{Log: recorded})
	transport := &http.Transport{Proxy: http.ProxyURL(proxyURL(t, srv))}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport}

	for range 2 {
		status, _, body, err := get(t, client, upstream.URL+"/again")
		if err != nil {
			t.Fatal(err)
		}
		if status != http.StatusOK || string(body) != "kept" {
			t.Fatalf("client saw %d %q", status, body)
		}
	}
	recorded.wait(t, func(tool.Flow) bool {
		flows := recorded.flowsNow()
		if len(flows) != 2 {
			return false
		}
		for _, flow := range flows {
			if flow.State != tool.FlowDone {
				return false
			}
		}
		return true
	})
}

func TestProxyUsesTheGivenDialer(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "dialed")
	}))
	t.Cleanup(upstream.Close)
	var calls atomic.Int32
	dialer := &net.Dialer{}
	srv := startProxy(t, Options{
		Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
			calls.Add(1)
			return dialer.DialContext(ctx, network, addr)
		},
	})
	_, _, body, err := get(t, clientThrough(t, srv, nil, false), upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "dialed" {
		t.Fatalf("client body = %q", body)
	}
	if calls.Load() != 1 {
		t.Errorf("dial was called %d times, want once for the upstream", calls.Load())
	}
}

func TestBodyCaptureStopsAtTheLimit(t *testing.T) {
	data := bytes.Repeat([]byte("x"), tool.MaxBody+10)
	capture := newBodyCapture(bytes.NewReader(data))
	read, err := io.ReadAll(capture)
	if err != nil {
		t.Fatal(err)
	}
	if len(read) != len(data) {
		t.Fatalf("read %d bytes, want the whole body %d", len(read), len(data))
	}
	body := capture.body("application/json; charset=utf-8", "")
	if len(body.Data) != tool.MaxBody {
		t.Errorf("kept %d bytes, want the limit %d", len(body.Data), tool.MaxBody)
	}
	if !body.Truncated {
		t.Error("body is not marked truncated")
	}
	if body.Size != int64(len(data)) {
		t.Errorf("size = %d, want the whole body %d", body.Size, len(data))
	}
	if body.Type != "application/json" {
		t.Errorf("type = %q, want application/json", body.Type)
	}
}

func TestDecodeBodyLeavesUnknownEncodings(t *testing.T) {
	data := []byte("\x28\xb5\x2f\xfdzstd-ish")
	decoded, cut := decodeBody(data, "zstd")
	if !bytes.Equal(decoded, data) || cut {
		t.Errorf("decodeBody(zstd) = %q, %v, want the bytes unchanged", decoded, cut)
	}
	decoded, _ = decodeBody([]byte("plain"), "identity")
	if string(decoded) != "plain" {
		t.Errorf("decodeBody(identity) = %q", decoded)
	}
}

func TestDecodeBodyUndoesBrotli(t *testing.T) {
	var compressed bytes.Buffer
	writer := brotli.NewWriter(&compressed)
	if _, err := writer.Write([]byte("hello brotli")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	decoded, cut := decodeBody(compressed.Bytes(), "br")
	if string(decoded) != "hello brotli" || cut {
		t.Errorf("decodeBody(br) = %q, cut %v", decoded, cut)
	}
}

func TestDecodeBodyUndoesChainedEncodings(t *testing.T) {
	// "gzip, br" means gzip was applied first, so br comes off first.
	plain := []byte("chained")
	var gzipped bytes.Buffer
	gzipWriter := gzip.NewWriter(&gzipped)
	if _, err := gzipWriter.Write(plain); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	var compressed bytes.Buffer
	brotliWriter := brotli.NewWriter(&compressed)
	if _, err := brotliWriter.Write(gzipped.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := brotliWriter.Close(); err != nil {
		t.Fatal(err)
	}
	decoded, cut := decodeBody(compressed.Bytes(), "gzip, br")
	if string(decoded) != "chained" || cut {
		t.Errorf("decodeBody(gzip, br) = %q, cut %v", decoded, cut)
	}
}

func TestDecodeBodyStopsABrotliBombAtTheCap(t *testing.T) {
	payload := bytes.Repeat([]byte("A"), tool.MaxBody+4096)
	var compressed bytes.Buffer
	writer := brotli.NewWriter(&compressed)
	if _, err := writer.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if compressed.Len() > 64<<10 {
		t.Fatalf("the bomb is not small: %d bytes", compressed.Len())
	}
	decoded, cut := decodeBody(compressed.Bytes(), "br")
	if len(decoded) != tool.MaxBody {
		t.Errorf("decoded %d bytes, want the cap %d", len(decoded), tool.MaxBody)
	}
	if !cut {
		t.Error("an oversized decoded body is not marked as cut")
	}
}

func TestMediaTypeDropsParameters(t *testing.T) {
	if got := mediaType("text/html; charset=utf-8"); got != "text/html" {
		t.Errorf("mediaType = %q", got)
	}
	if got := mediaType(""); got != "" {
		t.Errorf("mediaType of nothing = %q", got)
	}
}

func TestParseHeadKeepsOrderAndCase(t *testing.T) {
	raw := []byte("GET /a HTTP/1.1\r\nHost: example.test\r\nx-lower: one\r\nX-Upper: two\r\n\r\n")
	headers := parseHead(raw)
	if len(headers) != 3 {
		t.Fatalf("parsed %d headers: %v", len(headers), headers)
	}
	if headers[0].Name != "Host" || headers[1].Name != "x-lower" || headers[2].Name != "X-Upper" {
		t.Errorf("names are out of order or lowercased: %v", headers)
	}
	if headers[1].Value != "one" || headers[2].Value != "two" {
		t.Errorf("values = %v", headers)
	}
	if got := parseHead(nil); got != nil {
		t.Errorf("parseHead(nil) = %v", got)
	}
}
