package proxy

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// TestResponseHeadTimeoutEndsAHungServer checks that a server which never
// starts its answer does not keep a request and its connections forever.
func TestResponseHeadTimeoutEndsAHungServer(t *testing.T) {
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer func() { _ = conn.Close() }()
				_, _ = http.ReadRequest(bufio.NewReader(conn))
				_, _ = io.Copy(io.Discard, conn)
			}(conn)
		}
	}()

	recorded := newTestRecorder()
	srv := startProxyTuned(t, Options{Log: recorded}, func(s *Server) {
		s.responseHeadTimeout = 150 * time.Millisecond
	})
	start := time.Now()
	status, _, _, err := get(t, clientThrough(t, srv, nil, false), "http://"+listener.Addr().String()+"/")
	if err == nil && status != http.StatusBadGateway {
		t.Fatalf("client saw %d, want the 502 of a failed exchange", status)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("the exchange took %v to give up", elapsed)
	}
	flow := recorded.wait(t, func(flow tool.Flow) bool { return flow.State == tool.FlowFailed })
	if flow.Info == "" {
		t.Error("the failed flow has no error")
	}
}

// TestInterimResponsesAreCapped checks that a server that only sends 1xx
// answers ends the exchange instead of looping forever.
func TestInterimResponsesAreCapped(t *testing.T) {
	addr := startInterimLoop(t)
	recorded := newTestRecorder()
	srv := startProxy(t, Options{Log: recorded})
	status, _, _, err := get(t, clientThrough(t, srv, nil, false), "http://"+addr.String()+"/")
	if err == nil && status != http.StatusBadGateway {
		t.Fatalf("client saw %d, want the 502 of a failed exchange", status)
	}
	flow := recorded.wait(t, func(flow tool.Flow) bool { return flow.State == tool.FlowFailed })
	if !strings.Contains(flow.Info, "interim") {
		t.Errorf("flow info = %q, want the interim limit", flow.Info)
	}
}

// startInterimLoop runs a server that sends interim answers and then a real
// one.
func startInterimLoop(t *testing.T) net.Addr {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go answerInterimLoop(conn)
		}
	}()
	return listener.Addr()
}

// answerInterimLoop sends more interim answers than the proxy waits through,
// then the real response.
func answerInterimLoop(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err := http.ReadRequest(bufio.NewReader(conn)); err != nil {
		return
	}
	for range 12 {
		if _, err := io.WriteString(conn, "HTTP/1.1 100 Continue\r\n\r\n"); err != nil {
			return
		}
	}
	_, _ = io.WriteString(conn, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
}

// TestStalledTunnelHandshakeIsClosed checks that a client that sends CONNECT
// and then nothing is dropped instead of holding a connection for free.
func TestStalledTunnelHandshakeIsClosed(t *testing.T) {
	srv := startProxyTuned(t, Options{Decrypt: true, Authority: proxyCA(t)}, func(s *Server) {
		s.handshakeTimeout = 150 * time.Millisecond
	})
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(context.Background(), "tcp", srv.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err := io.WriteString(conn, "CONNECT 127.0.0.1:8443 HTTP/1.1\r\nHost: 127.0.0.1:8443\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(conn)
	if resp, err := http.ReadResponse(reader, &http.Request{Method: http.MethodConnect}); err != nil { //nolint:bodyclose // the body is the tunnel that follows.
		t.Fatal(err)
	} else if resp.StatusCode != http.StatusOK {
		t.Fatalf("CONNECT answered %d", resp.StatusCode)
	}

	start := time.Now()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, err = reader.ReadByte()
	if err == nil {
		t.Fatal("the stalled tunnel still carries data")
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		t.Fatalf("the proxy kept a stalled handshake open for %v", time.Since(start))
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("the stalled handshake took %v to close", elapsed)
	}
}

func TestUpgradeToAnH2CapableServer(t *testing.T) {
	upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "reached over h2", http.StatusBadGateway)
			return
		}
		conn, buffered, err := hijacker.Hijack()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_, _ = io.WriteString(conn, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: echo\r\nConnection: Upgrade\r\n\r\n")
		line, _ := buffered.ReadString('\n')
		_, _ = io.WriteString(conn, "echo:"+line)
	}))
	upstream.TLS = &tls.Config{
		Certificates: []tls.Certificate{testRoot.leaf},
		NextProtos:   []string{"h2", "http/1.1"},
	}
	upstream.EnableHTTP2 = true
	upstream.StartTLS()
	t.Cleanup(upstream.Close)

	authority := proxyCA(t)
	srv := startProxy(t, Options{Decrypt: true, Authority: authority})
	host := strings.TrimPrefix(upstream.URL, "https://")
	conn := connectTLS(t, srv, host, poolOf(authority))
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err := fmt.Fprintf(conn, "GET /ws HTTP/1.1\r\nHost: %s\r\nUpgrade: echo\r\nConnection: Upgrade\r\n\r\n", host); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("an upgrade to an h2 capable server answered %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
	if _, err := io.WriteString(conn, "ping\n"); err != nil {
		t.Fatal(err)
	}
	echo, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if echo != "echo:ping\n" {
		t.Fatalf("tunnel echoed %q", echo)
	}
}

// TestHTTP11ClientGetsAnHTTP11StatusLine checks that a response taken from
// an HTTP/2 server is written to an HTTP/1.1 client as HTTP/1.1.
func TestHTTP11ClientGetsAnHTTP11StatusLine(t *testing.T) {
	upstream := newUpstream(t, http.HandlerFunc(greet), true)
	authority := proxyCA(t)
	srv := startProxy(t, Options{Decrypt: true, Authority: authority})
	host := strings.TrimPrefix(upstream.URL, "https://")
	conn := connectTLS(t, srv, host, poolOf(authority))
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err := fmt.Fprintf(conn, "GET /x HTTP/1.1\r\nHost: %s\r\n\r\n", host); err != nil {
		t.Fatal(err)
	}
	status, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(status, "HTTP/1.1 ") {
		t.Fatalf("status line = %q, want HTTP/1.1", status)
	}
}

// TestRootCAsVerifyTheUpstream checks that the configured pool verifies an
// h2 server and that Do goes through the same check.
func TestRootCAsVerifyTheUpstream(t *testing.T) {
	upstream := newUpstream(t, http.HandlerFunc(greet), true)
	recorded := newTestRecorder()
	srv := startProxy(t, Options{Log: recorded, RootCAs: testRoot.roots()})

	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, upstream.URL+"/replayed", nil)
	if err != nil {
		t.Fatal(err)
	}
	id, err := srv.Do(context.Background(), request, 0)
	if err != nil {
		t.Fatal(err)
	}
	flow := recorded.wait(t, func(flow tool.Flow) bool { return flow.ID == id && flow.State == tool.FlowDone })
	if flow.Status != http.StatusOK {
		t.Errorf("status = %d, want 200", flow.Status)
	}
	if flow.TLSVersion == "" || flow.ServerAddr == "" {
		t.Errorf("flow does not record the TLS server: %q %q", flow.TLSVersion, flow.ServerAddr)
	}
	if string(flow.Response.Body.Data) != "hello through tls" {
		t.Errorf("recorded body = %q", flow.Response.Body.Data)
	}
}
