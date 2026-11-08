package proxy

import (
	"context"
	"crypto/tls"
	"fmt"
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
)

// recordingProxy is a stand-in for a corporate HTTP proxy: it records what
// it is asked and answers itself.
type recordingProxy struct {
	mu      sync.Mutex
	lines   []string
	auths   []string
	methods []string
}

// round is everything it saw.
func (p *recordingProxy) round() (lines, auths, methods []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.lines...), append([]string(nil), p.auths...), append([]string(nil), p.methods...)
}

func (p *recordingProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	p.lines = append(p.lines, r.RequestURI)
	p.auths = append(p.auths, r.Header.Get("Proxy-Authorization"))
	p.methods = append(p.methods, r.Method)
	p.mu.Unlock()
	w.Header().Set("X-Via", "upstream")
	_, _ = fmt.Fprintf(w, "proxied %s", r.RequestURI)
}

func TestPlainRequestsGoThroughTheUpstreamProxy(t *testing.T) {
	fake := &recordingProxy{}
	upstream := httptest.NewServer(fake)
	t.Cleanup(upstream.Close)
	proxyURL, err := url.Parse("http://user:secret@" + strings.TrimPrefix(upstream.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	recorded := newTestRecorder()
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	var dials atomic.Int32
	srv := startProxy(t, Options{
		Log:           recorded,
		RootCAs:       testRoot.roots(),
		UpstreamProxy: proxyURL,
		Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
			dials.Add(1)
			return dialer.DialContext(ctx, network, addr)
		},
	})
	client := clientThrough(t, srv, nil, false)

	target := "http://origin.invalid/thing?q=1"
	status, header, body, err := get(t, client, target)
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || header.Get("X-Via") != "upstream" {
		t.Fatalf("client saw %d %v %q", status, header, body)
	}
	lines, auths, methods := fake.round()
	if len(lines) != 1 || lines[0] != target {
		t.Errorf("upstream proxy saw request lines %v, want the absolute target", lines)
	}
	if len(auths) != 1 || auths[0] != "Basic dXNlcjpzZWNyZXQ=" {
		t.Errorf("upstream proxy saw credentials %v", auths)
	}
	if len(methods) != 1 || methods[0] != http.MethodGet {
		t.Errorf("upstream proxy saw methods %v, want a plain GET without CONNECT", methods)
	}
	if got := dials.Load(); got != 0 {
		t.Errorf("the tunnel dialer was used %d times for a plain request", got)
	}
	flow := recorded.wait(t, func(flow tool.Flow) bool { return flow.State == tool.FlowDone })
	if flow.URL != target {
		t.Errorf("recorded URL = %q, want the target", flow.URL)
	}
	if flow.ServerAddr != strings.TrimPrefix(upstream.URL, "http://") {
		t.Errorf("server address = %q, want the upstream proxy", flow.ServerAddr)
	}
}

// TestTLSTunnelsIgnoreTheUpstreamProxy checks that an https request is not
// given to the plain HTTP proxy: only the tunnel dialer reaches it.
func TestTLSTunnelsIgnoreTheUpstreamProxy(t *testing.T) {
	fake := &recordingProxy{}
	upstreamProxy := httptest.NewServer(fake)
	t.Cleanup(upstreamProxy.Close)
	proxyURL, err := url.Parse(upstreamProxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	upstream := newUpstream(t, http.HandlerFunc(greet), false)
	authority := proxyCA(t)
	srv := startProxy(t, Options{
		Decrypt:       true,
		Authority:     authority,
		UpstreamProxy: proxyURL,
	})
	client := clientThrough(t, srv, &tls.Config{
		RootCAs:    poolOf(authority),
		NextProtos: []string{"http/1.1"},
	}, false)

	status, _, body, err := get(t, client, upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || string(body) != "hello through tls" {
		t.Fatalf("client saw %d %q", status, body)
	}
	if _, auths, methods := fake.round(); len(methods) != 0 || len(auths) != 0 {
		t.Errorf("the plain upstream proxy saw %v %v for an https request", methods, auths)
	}
}

func TestNewRejectsABadUpstreamProxy(t *testing.T) {
	for _, bad := range []struct {
		name   string
		target *url.URL
	}{
		{"scheme", &url.URL{Scheme: "https", Host: "127.0.0.1:3128"}},
		{"itself", &url.URL{Scheme: "http", Host: "127.0.0.1:19099"}},
		{"no host", &url.URL{Scheme: "http"}},
	} {
		if _, err := New(Options{Addr: "127.0.0.1:19099", UpstreamProxy: bad.target}); err == nil {
			t.Errorf("%s upstream proxy was accepted", bad.name)
		}
	}
	good := &url.URL{Scheme: "http", Host: "127.0.0.1:3128"}
	if _, err := New(Options{Addr: "127.0.0.1:19099", UpstreamProxy: good}); err != nil {
		t.Errorf("a good upstream proxy was refused: %v", err)
	}
}
