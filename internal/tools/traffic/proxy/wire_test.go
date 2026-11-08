package proxy

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// wireListener records every byte its connections read from the network,
// which is the ciphertext when a TLS server reads through it.
type wireListener struct {
	net.Listener
	mu   sync.Mutex
	data bytes.Buffer
}

// Accept wraps the accepted connection so its reads are recorded.
func (l *wireListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &wireConn{Conn: conn, listener: l}, nil
}

// wire is everything the listener has seen, so far.
func (l *wireListener) wire() []byte {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]byte(nil), l.data.Bytes()...)
}

// wireConn records what the server reads from one connection.
type wireConn struct {
	net.Conn
	listener *wireListener
}

// Read records the bytes the server reads.
func (c *wireConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 {
		c.listener.mu.Lock()
		c.listener.data.Write(p[:n])
		c.listener.mu.Unlock()
	}
	return n, err
}

// newWireUpstream is an HTTPS server whose socket bytes the test can read.
func newWireUpstream(t *testing.T, handler http.Handler, http2 bool) (*httptest.Server, *wireListener) {
	t.Helper()
	srv := httptest.NewUnstartedServer(handler)
	wire := &wireListener{Listener: srv.Listener}
	srv.Listener = wire
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{testRoot.leaf}}
	srv.EnableHTTP2 = http2
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv, wire
}

// connectTLS opens a CONNECT tunnel through srv and hands back the TLS
// connection inside it, speaking HTTP/1.1.
func connectTLS(t *testing.T, srv *Server, authority string, roots *x509.CertPool) *tls.Conn {
	t.Helper()
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(context.Background(), "tcp", srv.Addr())
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	t.Cleanup(func() { _ = conn.Close() })
	if _, err := fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", authority, authority); err != nil {
		t.Fatal(err)
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodConnect}) //nolint:bodyclose // the body is the tunnel, which the TLS layer reads next.
	if err != nil {
		t.Fatalf("read the CONNECT answer: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("CONNECT answered %d", resp.StatusCode)
	}
	host, _, err := net.SplitHostPort(authority)
	if err != nil {
		t.Fatal(err)
	}
	inner := tls.Client(&bufferedConn{Conn: conn, r: br}, &tls.Config{
		ServerName: host,
		RootCAs:    roots,
		NextProtos: []string{"http/1.1"},
	})
	if err := inner.HandshakeContext(context.Background()); err != nil {
		t.Fatalf("handshake inside the tunnel: %v", err)
	}
	return inner
}

// TestUpgradeOverTLSDoesNotLeakPlaintext checks that a WebSocket over a
// decrypted tunnel is piped inside the upstream TLS session, not on the raw
// socket.
func TestUpgradeOverTLSDoesNotLeakPlaintext(t *testing.T) {
	upstream, wire := newWireUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Upgrade") == "" {
			http.Error(w, "no upgrade", http.StatusBadRequest)
			return
		}
		conn, buffered, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_, _ = io.WriteString(conn, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: echo\r\nConnection: Upgrade\r\n\r\n")
		line, _ := buffered.ReadString('\n')
		_, _ = io.WriteString(conn, "echo:"+line)
	}), false)

	authority := proxyCA(t)
	srv := startProxy(t, Options{Decrypt: true, Authority: authority})
	host := strings.TrimPrefix(upstream.URL, "https://")
	conn := connectTLS(t, srv, host, poolOf(authority))
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))

	const token = "SECRET-FRAME-9c1f2e"
	request := fmt.Sprintf("GET /ws HTTP/1.1\r\nHost: %s\r\nUpgrade: echo\r\nConnection: Upgrade\r\n\r\n", host)
	if _, err := io.WriteString(conn, request); err != nil {
		t.Fatal(err)
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil) //nolint:bodyclose // the body is the upgraded tunnel.
	if err != nil {
		t.Fatalf("read the upgrade answer: %v", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("status = %d, want 101", resp.StatusCode)
	}
	if _, err := io.WriteString(conn, token+"\n"); err != nil {
		t.Fatal(err)
	}
	echo, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("read the echoed frame: %v (wire held %d bytes)", err, len(wire.wire()))
	}
	if echo != "echo:"+token+"\n" {
		t.Fatalf("tunnel echoed %q", echo)
	}
	if bytes.Contains(wire.wire(), []byte(token)) {
		t.Fatal("the frame crossed the network in clear")
	}
}

// TestEarlyResponseDoesNotSmuggleTheBody checks that a decrypted HTTP/1.1
// request whose body the upstream never read cannot be reused.
func TestEarlyResponseDoesNotSmuggleTheBody(t *testing.T) {
	var upstreamRequests atomic.Int32
	upstream := newUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamRequests.Add(1)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, "no")
	}), true)

	authority := proxyCA(t)
	srv := startProxy(t, Options{Decrypt: true, Authority: authority})
	host := strings.TrimPrefix(upstream.URL, "https://")
	conn := connectTLS(t, srv, host, poolOf(authority))
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))

	body := bytes.Repeat([]byte("A"), 2<<20)
	if _, err := fmt.Fprintf(conn, "POST /upload HTTP/1.1\r\nHost: %s\r\nContent-Length: %d\r\n\r\n", host, len(body)); err != nil {
		t.Fatal(err)
	}
	// The proxy may close the connection before the whole body is out,
	// because the upstream answered without reading it.
	if _, err := conn.Write(body); err != nil {
		t.Logf("body write stopped early: %v", err)
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("read the early answer: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}

	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	_, _ = io.WriteString(conn, "GET /second HTTP/1.1\r\nHost: "+host+"\r\n\r\n")
	line, err := br.ReadString('\n')
	if err == nil && strings.Contains(line, "200") {
		t.Fatal("the proxy answered a second request though the first body was not read")
	}
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			t.Fatal("the connection stayed open after a response whose request body was not read")
		}
	}
	if got := upstreamRequests.Load(); got != 1 {
		t.Fatalf("the upstream got %d requests, want one", got)
	}
}

// TestRuleStatus101WithoutUpstreamConnDoesNotCrash checks that a rule that
// turns a response into 101 over an h2 upstream does not pipe a nil
// connection.
func TestRuleStatus101WithoutUpstreamConnDoesNotCrash(t *testing.T) {
	upstream := newUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "hello")
	}), true)
	recorded := newTestRecorder()
	ruler := &testRuler{}
	ruler.response = func(_ *http.Request, resp *http.Response) {
		resp.StatusCode = http.StatusSwitchingProtocols
		resp.Status = "101 Switching Protocols"
		resp.ContentLength = 0
		resp.Body = io.NopCloser(strings.NewReader(""))
	}
	authority := proxyCA(t)
	srv := startProxy(t, Options{Log: recorded, Decrypt: true, Authority: authority, Rules: ruler})
	host := strings.TrimPrefix(upstream.URL, "https://")
	conn := connectTLS(t, srv, host, poolOf(authority))
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))

	if _, err := fmt.Fprintf(conn, "GET /x HTTP/1.1\r\nHost: %s\r\n\r\n", host); err != nil {
		t.Fatal(err)
	}
	br := bufio.NewReader(conn)
	status, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("read the answer: %v", err)
	}
	if !strings.Contains(status, "101") {
		t.Fatalf("status line = %q, want 101", status)
	}
	flow := recorded.wait(t, func(flow tool.Flow) bool { return flow.Status == http.StatusSwitchingProtocols })
	if flow.State != tool.FlowDone {
		t.Errorf("flow state = %v", flow.State)
	}
}
