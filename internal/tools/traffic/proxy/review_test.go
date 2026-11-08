package proxy

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/tools/traffic/rules"
)

// nestedGzip wraps "x" in layers of stored gzip, the way a hostile server
// can, and returns the matching Content-Encoding header.
func nestedGzip(t *testing.T, layers int) ([]byte, string) {
	t.Helper()
	data := []byte("x")
	for range layers {
		data = storedGzip(t, data)
	}
	return data, strings.TrimSuffix(strings.Repeat("gzip,", layers), ",")
}

// storedGzip gzips data without compressing it, so each layer stays small.
func storedGzip(t *testing.T, data []byte) []byte {
	t.Helper()
	var compressed bytes.Buffer
	writer, err := gzip.NewWriterLevel(&compressed, gzip.NoCompression)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return append([]byte(nil), compressed.Bytes()...)
}

func TestRequestDropsEveryDecodedEncoding(t *testing.T) {
	for _, encoding := range []string{"gzip", "x-gzip", "deflate", "br", "gzip, br"} {
		flow := tool.Flow{
			FlowRow: tool.FlowRow{ID: 1, Method: http.MethodPost, Host: "example.test"},
			URL:     "http://example.test/",
			Request: tool.HTTPMessage{
				Headers: []tool.Header{{Name: "Content-Encoding", Value: encoding}},
				Body:    tool.Body{Data: []byte("plain")},
			},
		}
		request, err := Request(flow)
		if err != nil {
			t.Fatal(err)
		}
		if got := request.Header.Get("Content-Encoding"); got != "" {
			t.Errorf("replay of %q kept Content-Encoding %q over the decoded body", encoding, got)
		}
	}
	for _, encoding := range []string{"zstd", "gzip, zstd", "gzip, gzip, gzip, gzip, gzip"} {
		flow := tool.Flow{
			FlowRow: tool.FlowRow{ID: 1, Method: http.MethodPost, Host: "example.test"},
			URL:     "http://example.test/",
			Request: tool.HTTPMessage{
				Headers: []tool.Header{{Name: "Content-Encoding", Value: encoding}},
			},
		}
		request, err := Request(flow)
		if err != nil {
			t.Fatal(err)
		}
		if got := request.Header.Get("Content-Encoding"); got != encoding {
			t.Errorf("replay of %q changed Content-Encoding to %q", encoding, got)
		}
	}
}

func TestDecodeBodyGivesUpOnALongChain(t *testing.T) {
	body, encoding := nestedGzip(t, 5)
	decoded, cut := decodeBody(body, encoding)
	if !bytes.Equal(decoded, body) || cut {
		t.Fatalf("a five layer chain was decoded (%d bytes, cut %v); the raw bytes must be kept", len(decoded), cut)
	}
}

func TestDecodeBodyBoundsTheDecodedBytesOfAChain(t *testing.T) {
	body := bytes.Repeat([]byte("A"), 2<<20)
	for range 3 {
		body = storedGzip(t, body)
	}
	decoded, cut := decodeBody(body, "gzip, gzip, gzip")
	if !bytes.Equal(decoded, body) || cut {
		t.Fatalf("a chain decoding %d bytes was not refused (cut %v)", len(decoded), cut)
	}
}

func TestHostileEncodingChainIsRecordedRaw(t *testing.T) {
	body, encoding := nestedGzip(t, 5)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Encoding", encoding)
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write(body)
	}))
	t.Cleanup(upstream.Close)
	recorded := newTestRecorder()
	srv := startProxy(t, Options{Log: recorded})
	transport := &http.Transport{
		Proxy:              http.ProxyURL(proxyURL(t, srv)),
		DisableCompression: true,
		DisableKeepAlives:  true,
	}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}

	_, _, answer, err := get(t, client, upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(answer, body) {
		t.Fatal("the client body differs from the bytes the server sent")
	}
	flow := recorded.wait(t, func(flow tool.Flow) bool { return flow.State == tool.FlowDone })
	if !bytes.Equal(flow.Response.Body.Data, body) {
		t.Errorf("recorded body was decoded (%d bytes), want the raw %d", len(flow.Response.Body.Data), len(body))
	}
	if got := headerValue(flow.Response.Headers, "Content-Encoding"); got != encoding {
		t.Errorf("recorded headers lost Content-Encoding: %q", got)
	}
}

func TestUpgradeStripsProxyHeaders(t *testing.T) {
	var proxyAuth, proxyConnection, upgrade, connection atomic.Value
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyAuth.Store(r.Header.Get("Proxy-Authorization"))
		proxyConnection.Store(r.Header.Get("Proxy-Connection"))
		upgrade.Store(r.Header.Get("Upgrade"))
		connection.Store(r.Header.Get("Connection"))
		http.Error(w, "no", http.StatusBadRequest)
	}))
	t.Cleanup(upstream.Close)
	srv := startProxy(t, Options{})
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(t.Context(), "tcp", srv.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	host := strings.TrimPrefix(upstream.URL, "http://")
	request := "GET %s/ws HTTP/1.1\r\nHost: %s\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n" +
		"Proxy-Authorization: Basic dXNlcjpzZWNyZXQ=\r\nProxy-Connection: keep-alive\r\n\r\n"
	if _, err := fmt.Fprintf(conn, request, upstream.URL, host); err != nil {
		t.Fatal(err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if got := proxyAuth.Load(); got != nil && got != "" {
		t.Errorf("Proxy-Authorization reached the website: %q", got)
	}
	if got := proxyConnection.Load(); got != nil && got != "" {
		t.Errorf("Proxy-Connection reached the website: %q", got)
	}
	if got, _ := upgrade.Load().(string); got != "websocket" {
		t.Errorf("Upgrade = %q, want websocket", got)
	}
	if got, _ := connection.Load().(string); !strings.Contains(strings.ToLower(got), "upgrade") {
		t.Errorf("Connection = %q, want an Upgrade token", got)
	}
}

func TestTeeRecordsOnlyWhileReadingAHead(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { _ = client.Close(); _ = server.Close() })
	go func() {
		_, _ = server.Write([]byte("a\r\n\r\n"))
		_, _ = server.Write([]byte("b\r\n\r\n"))
	}()
	tee := &teeConn{Conn: client}
	tee.begin()
	head := make([]byte, len("a\r\n\r\n"))
	if _, err := io.ReadFull(tee, head); err != nil {
		t.Fatal(err)
	}
	tee.lift()
	rest := make([]byte, len("b\r\n\r\n"))
	if _, err := io.ReadFull(tee, rest); err != nil {
		t.Fatal(err)
	}
	if got := string(tee.headFrom(0)); got != "a\r\n\r\n" {
		t.Fatalf("recorded head = %q", got)
	}
	if got := tee.headFrom(int64(len(head))); got != nil {
		t.Fatalf("the tee recorded %q after the head was read", got)
	}
}

// panickingRuler fails every request the way a bug in a rule could.
type panickingRuler struct{}

// Request panics.
func (panickingRuler) Request(*http.Request) (rules.Verdict, error) { panic("rule blew up") }

// Response changes nothing.
func (panickingRuler) Response(*http.Request, *http.Response) ([]string, error) {
	return nil, nil
}

func TestPanicFinishesTheOpenFlow(t *testing.T) {
	recorded := newTestRecorder()
	srv := startProxy(t, Options{Log: recorded, Rules: panickingRuler{}})
	_, _, _, _ = get(t, clientThrough(t, srv, nil, false), "http://example.test/")

	flow := recorded.wait(t, func(flow tool.Flow) bool { return flow.State != tool.FlowOpen })
	if flow.State != tool.FlowFailed {
		t.Fatalf("state = %v, want failed", flow.State)
	}
	if !strings.Contains(flow.Info, "proxy error") {
		t.Errorf("info = %q, want the panic", flow.Info)
	}
	time.Sleep(50 * time.Millisecond)
	if flows := recorded.flowsNow(); len(flows) != 1 {
		t.Fatalf("recorded %d flows, want the one that was open finished", len(flows))
	}
}
