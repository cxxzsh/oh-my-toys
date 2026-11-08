package proxy

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/tools/traffic/rules"
)

// urlRewriteRuler sends every request to target, as a URL rewrite rule does.
type urlRewriteRuler struct {
	mu     sync.Mutex
	target string
}

// set points the ruler at target.
func (r *urlRewriteRuler) set(target string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.target = target
}

// Request points the request at the ruler's target.
func (r *urlRewriteRuler) Request(req *http.Request) (rules.Verdict, error) {
	r.mu.Lock()
	target := r.target
	r.mu.Unlock()
	rewritten, err := url.Parse(target)
	if err != nil {
		return rules.Verdict{}, err
	}
	req.URL = rewritten
	return rules.Verdict{}, nil
}

// Response changes nothing.
func (*urlRewriteRuler) Response(*http.Request, *http.Response) ([]string, error) {
	return nil, nil
}

func TestIsSelfRecognizesLocalNames(t *testing.T) {
	srv := startProxy(t, Options{})
	port := srv.port()
	for _, target := range []string{
		"http://localhost:" + port + "/",
		"http://localhost.:" + port + "/",
		"http://127.0.0.1:" + port + "/",
		"http://127.1:" + port + "/",
		"http://0.0.0.0:" + port + "/",
		"http://[::1]:" + port + "/",
		"https://127.0.0.1:" + port + "/",
	} {
		parsed, err := url.Parse(target)
		if err != nil {
			t.Fatal(err)
		}
		if !srv.isSelf(context.Background(), parsed) {
			t.Errorf("isSelf(%s) = false, want true", target)
		}
	}
	other, err := url.Parse("http://127.0.0.1:1/")
	if err != nil {
		t.Fatal(err)
	}
	if srv.isSelf(context.Background(), other) {
		t.Error("isSelf accepted another port")
	}
}

func TestConnectToTheProxyItselfIsRefused(t *testing.T) {
	srv := startProxy(t, Options{})
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(context.Background(), "tcp", srv.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	target := srv.Addr()
	if _, err := fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(line, "508") {
		t.Fatalf("status line = %q, want 508", line)
	}
}

func TestRewrittenURLToTheProxyIsRefused(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "upstream")
	}))
	t.Cleanup(upstream.Close)
	ruler := &urlRewriteRuler{}
	srv := startProxy(t, Options{Rules: ruler})
	ruler.set("http://" + srv.Addr() + "/loop")

	status, _, _, err := get(t, clientThrough(t, srv, nil, false), upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusLoopDetected {
		t.Fatalf("status = %d, want 508 for a rewritten URL that targets the proxy", status)
	}
}

func TestDecryptedRequestForAnotherHostIsRejected(t *testing.T) {
	var upstreamRequests atomic.Int32
	upstream := newUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamRequests.Add(1)
		_, _ = io.WriteString(w, "upstream")
	}), false)
	authority := proxyCA(t)
	srv := startProxy(t, Options{Decrypt: true, Authority: authority})
	host := strings.TrimPrefix(upstream.URL, "https://")
	conn := connectTLS(t, srv, host, poolOf(authority))
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))

	if _, err := io.WriteString(conn, "GET /other HTTP/1.1\r\nHost: no-such-host.invalid\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(line, "421") {
		t.Fatalf("status line = %q, want 421", line)
	}
	if got := upstreamRequests.Load(); got != 0 {
		t.Fatalf("the upstream got %d requests, want none", got)
	}
}

func TestMatchHostIgnoresATrailingDot(t *testing.T) {
	if !matchHost("bank.com", "bank.com.") {
		t.Error("Bypass bank.com does not match bank.com.")
	}
	if !matchHost("bank.com.", "bank.com") {
		t.Error("Bypass bank.com. does not match bank.com")
	}
	if !matchHost("*.example.com", "TLS.Example.COM.") {
		t.Error("a glob did not match ignoring case and a trailing dot")
	}
	if matchHost("bank.com", "notbank.com") {
		t.Error("a different host matched")
	}
}

// TestReplayDropsTheDecodedContentEncoding checks that the request rebuilt
// from a flow does not claim a content encoding whose bytes were decoded.
func TestReplayDropsTheDecodedContentEncoding(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		_, _ = fmt.Fprintf(w, "ce=%s body=%s", req.Header.Get("Content-Encoding"), body)
	}))
	t.Cleanup(upstream.Close)
	recorded := newTestRecorder()
	srv := startProxy(t, Options{Log: recorded})
	client := clientThrough(t, srv, nil, false)

	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, _ = writer.Write([]byte("payload"))
	_ = writer.Close()
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, upstream.URL, bytes.NewReader(compressed.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Encoding", "gzip")
	resp, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	flow := recorded.wait(t, func(flow tool.Flow) bool { return flow.State == tool.FlowDone })
	if string(flow.Request.Body.Data) != "payload" {
		t.Fatalf("recorded body = %q, want the decoded text", flow.Request.Body.Data)
	}
	again, err := Request(flow)
	if err != nil {
		t.Fatal(err)
	}
	if got := again.Header.Get("Content-Encoding"); got != "" {
		t.Fatalf("replayed request still says Content-Encoding: %s", got)
	}
	id, err := srv.Do(context.Background(), again, flow.ID)
	if err != nil {
		t.Fatal(err)
	}
	replayed := recorded.wait(t, func(flow tool.Flow) bool { return flow.ID == id && flow.State == tool.FlowDone })
	if got := string(replayed.Response.Body.Data); got != "ce= body=payload" {
		t.Fatalf("the server saw %q, want the decoded body with no encoding", got)
	}
}
