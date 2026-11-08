package proxy

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/tools/traffic/ca"
)

// poolOf is a certificate pool trusting only one proxy root.
func poolOf(authority *ca.Authority) *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(authority.CertPEM())
	return pool
}

// greet is the handler the HTTPS tests use.
func greet(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("X-Server", "upstream")
	_, _ = io.WriteString(w, "hello through tls")
}

func TestProxyDecryptsHTTPSOverHTTP1(t *testing.T) {
	upstream := newUpstream(t, http.HandlerFunc(greet), false)
	recorded := newTestRecorder()
	authority := proxyCA(t)
	srv := startProxy(t, Options{Log: recorded, Decrypt: true, Authority: authority})
	client := clientThrough(t, srv, &tls.Config{
		RootCAs:    poolOf(authority),
		NextProtos: []string{"http/1.1"},
	}, false)

	status, header, body, err := get(t, client, upstream.URL+"/x")
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || string(body) != "hello through tls" || header.Get("X-Server") != "upstream" {
		t.Fatalf("client saw %d %q %v", status, body, header)
	}
	flow := recorded.wait(t, func(flow tool.Flow) bool { return flow.State == tool.FlowDone })
	if !flow.Decrypted {
		t.Error("flow is not marked decrypted")
	}
	if flow.Proto != protoHTTP11 {
		t.Errorf("proto = %q, want HTTP/1.1", flow.Proto)
	}
	if flow.TLSVersion == "" {
		t.Error("TLS version is empty")
	}
	if flow.URL != upstream.URL+"/x" {
		t.Errorf("url = %q", flow.URL)
	}
	if string(flow.Response.Body.Data) != "hello through tls" {
		t.Errorf("recorded body = %q", flow.Response.Body.Data)
	}
}

func TestProxyDecryptsHTTPSOverHTTP2(t *testing.T) {
	var upstreamProto string
	upstream := newUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		upstreamProto = req.Proto
		greet(w, req)
	}), true)
	recorded := newTestRecorder()
	authority := proxyCA(t)
	srv := startProxy(t, Options{Log: recorded, Decrypt: true, Authority: authority})
	client := clientThrough(t, srv, &tls.Config{RootCAs: poolOf(authority)}, true)

	status, _, body, err := get(t, client, upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || string(body) != "hello through tls" {
		t.Fatalf("client saw %d %q", status, body)
	}
	flow := recorded.wait(t, func(flow tool.Flow) bool { return flow.State == tool.FlowDone })
	if flow.Proto != protoH2 {
		t.Errorf("flow proto = %q, want h2", flow.Proto)
	}
	if upstreamProto != "HTTP/2.0" {
		t.Errorf("upstream saw %q, want HTTP/2.0", upstreamProto)
	}
}

func TestProxyTunnelsWhenDecryptIsOff(t *testing.T) {
	upstream := newUpstream(t, http.HandlerFunc(greet), false)
	recorded := newTestRecorder()
	authority := proxyCA(t)
	srv := startProxy(t, Options{Log: recorded, Authority: authority, Decrypt: false})
	client := clientThrough(t, srv, &tls.Config{
		RootCAs:    testRoot.roots(),
		NextProtos: []string{"http/1.1"},
	}, false)

	_, _, body, err := get(t, client, upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "hello through tls" {
		t.Fatalf("client body = %q", body)
	}
	flow := recorded.wait(t, func(flow tool.Flow) bool {
		return flow.Proto == protoTunnel && flow.State == tool.FlowDone
	})
	if flow.Method != http.MethodConnect {
		t.Errorf("method = %q, want CONNECT", flow.Method)
	}
	if flow.Host != strings.TrimPrefix(upstream.URL, "https://") {
		t.Errorf("host = %q", flow.Host)
	}
	if flow.Size <= 0 {
		t.Errorf("tunnel size = %d, want the bytes both ways", flow.Size)
	}
	if flow.Decrypted || len(flow.Request.Headers) != 0 || len(flow.Response.Body.Data) != 0 {
		t.Errorf("tunnel flow holds an exchange: %+v", flow)
	}
	if flows := recorded.flowsNow(); len(flows) != 1 {
		t.Errorf("recorded %d flows, want one tunnel", len(flows))
	}
}

func TestProxyTunnelsABypassedHost(t *testing.T) {
	upstream := newUpstream(t, http.HandlerFunc(greet), false)
	recorded := newTestRecorder()
	srv := startProxy(t, Options{
		Log:       recorded,
		Decrypt:   true,
		Authority: proxyCA(t),
		Bypass:    []string{"192.0.2.1", "127.*"},
	})
	client := clientThrough(t, srv, &tls.Config{
		RootCAs:    testRoot.roots(),
		NextProtos: []string{"http/1.1"},
	}, false)

	_, _, body, err := get(t, client, upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "hello through tls" {
		t.Fatalf("client body = %q", body)
	}
	flow := recorded.wait(t, func(flow tool.Flow) bool {
		return flow.Proto == protoTunnel && flow.State == tool.FlowDone
	})
	if flow.Decrypted {
		t.Error("a bypassed host was decrypted")
	}
}

func TestProxyRecordsAnUntrustedClientAsFailed(t *testing.T) {
	upstream := newUpstream(t, http.HandlerFunc(greet), false)
	recorded := newTestRecorder()
	srv := startProxy(t, Options{Log: recorded, Decrypt: true, Authority: proxyCA(t)})
	client := clientThrough(t, srv, &tls.Config{
		RootCAs:    x509.NewCertPool(),
		NextProtos: []string{"http/1.1"},
	}, false)

	if _, _, _, err := get(t, client, upstream.URL); err == nil {
		t.Fatal("a client that trusts no root completed the request")
	}
	flow := recorded.wait(t, func(flow tool.Flow) bool { return flow.State == tool.FlowFailed })
	if !strings.Contains(flow.Info, "client did not trust the certificate") {
		t.Errorf("flow info = %q, want the trust sentence", flow.Info)
	}
	if flow.Method != http.MethodConnect {
		t.Errorf("method = %q, want CONNECT", flow.Method)
	}
}

// TestProxyKeepsAnUpgradeOpen checks the handshake is recorded and the bytes
// after it are piped without decoding.
func TestProxyKeepsAnUpgradeOpen(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		conn, buffered, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_, _ = io.WriteString(conn, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: echo\r\nConnection: Upgrade\r\n\r\n")
		line, _ := buffered.ReadString('\n')
		_, _ = io.WriteString(conn, "echo:"+line)
	}))
	t.Cleanup(upstream.Close)
	recorded := newTestRecorder()
	srv := startProxy(t, Options{Log: recorded})

	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(context.Background(), "tcp", srv.Addr())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	host := strings.TrimPrefix(upstream.URL, "http://")
	_, _ = fmt.Fprintf(conn, "GET %s/ws HTTP/1.1\r\nHost: %s\r\nUpgrade: echo\r\nConnection: Upgrade\r\n\r\n", upstream.URL, host)
	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("status = %d, want 101", resp.StatusCode)
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
		t.Fatalf("tunnel echoed %q, want %q", echo, "echo:ping\n")
	}
	flow := recorded.wait(t, func(flow tool.Flow) bool { return flow.Status == http.StatusSwitchingProtocols })
	if flow.Method != http.MethodGet || flow.URL != upstream.URL+"/ws" {
		t.Errorf("upgrade flow is %s %s", flow.Method, flow.URL)
	}
}

// TestProxyVerifiesTheUpstreamCertificate proves the proxy never weakens
// the check of the server it talks to: an upstream certificate that the
// system does not trust ends the flow.
func TestProxyVerifiesTheUpstreamCertificate(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(greet))
	t.Cleanup(upstream.Close)
	recorded := newTestRecorder()
	authority := proxyCA(t)
	srv := startProxy(t, Options{Log: recorded, Decrypt: true, Authority: authority})
	client := clientThrough(t, srv, &tls.Config{
		RootCAs:    poolOf(authority),
		NextProtos: []string{"http/1.1"},
	}, false)

	status, _, _, err := get(t, client, upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502 for an untrusted server", status)
	}
	flow := recorded.wait(t, func(flow tool.Flow) bool { return flow.State == tool.FlowFailed })
	if !strings.Contains(flow.Info, "certificate") {
		t.Errorf("flow info = %q, want the certificate error", flow.Info)
	}
}
