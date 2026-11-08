package traffic

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/cxxzsh/oh-my-toys/internal/platform/sysproxy"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

func TestChooseUpstream(t *testing.T) {
	previous := priorSettings()
	own := sysproxy.Snapshot{Services: []sysproxy.Service{{
		Name: "Wi-Fi",
		HTTP: sysproxy.Proxy{Enabled: true, Host: "127.0.0.1", Port: 3128},
	}}}
	nothing := sysproxy.Snapshot{}
	pac := sysproxy.Snapshot{Services: []sysproxy.Service{{
		Name: "Wi-Fi",
		Auto: sysproxy.AutoProxy{Enabled: true, URL: "http://pac.example/proxy.pac"},
	}}}
	tests := []struct {
		name    string
		cfg     tool.CaptureConfig
		before  *sysproxy.Snapshot
		want    string
		wantErr bool
	}{
		{
			name: "the person's choice wins",
			cfg:  tool.CaptureConfig{ProxyPort: 9099, Upstream: "socks5://10.0.0.2:1080"}, before: &previous,
			want: "socks5://10.0.0.2:1080",
		},
		{
			name: "the proxy the system had before",
			cfg:  tool.CaptureConfig{ProxyPort: 9099, SystemProxy: true}, before: &previous,
			want: "http://10.0.0.9:3128",
		},
		{
			name: "the capture's own address is no upstream",
			cfg:  tool.CaptureConfig{ProxyPort: 3128, SystemProxy: true}, before: &own,
		},
		{
			name: "the system had no proxy",
			cfg:  tool.CaptureConfig{ProxyPort: 9099, SystemProxy: true}, before: &nothing,
		},
		{
			name: "the system proxy is left alone",
			cfg:  tool.CaptureConfig{ProxyPort: 9099}, before: nil,
		},
		{
			name: "a PAC file cannot be followed",
			cfg:  tool.CaptureConfig{ProxyPort: 9099, SystemProxy: true}, before: &pac, wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := chooseUpstream(tt.cfg, tt.before)
			if tt.wantErr {
				if !errors.Is(err, ErrBadConfig) {
					t.Fatalf("chooseUpstream = %q, %v, want ErrBadConfig", got, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("chooseUpstream: %v", err)
			}
			if got != tt.want {
				t.Errorf("chooseUpstream = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSnapshotProxy(t *testing.T) {
	tests := []struct {
		name string
		snap sysproxy.Snapshot
		want string
	}{
		{
			name: "macOS HTTPS",
			snap: sysproxy.Snapshot{Services: []sysproxy.Service{{
				HTTPS: sysproxy.Proxy{Enabled: true, Host: "10.0.0.9", Port: 3128},
			}}},
			want: "10.0.0.9:3128",
		},
		{
			name: "macOS HTTP only",
			snap: sysproxy.Snapshot{Services: []sysproxy.Service{{
				HTTP: sysproxy.Proxy{Enabled: true, Host: "10.0.0.9", Port: 3128},
			}}},
			want: "10.0.0.9:3128",
		},
		{
			name: "macOS disabled",
			snap: sysproxy.Snapshot{Services: []sysproxy.Service{{
				HTTP: sysproxy.Proxy{Enabled: false, Host: "10.0.0.9", Port: 3128},
			}}},
			want: "",
		},
		{
			name: "Windows one address",
			snap: sysproxy.Snapshot{Windows: &sysproxy.WindowsSettings{
				ProxyEnable: sysproxy.DWORD{Present: true, Number: 1},
				ProxyServer: sysproxy.StringValue{Present: true, Text: "10.0.0.9:8080"},
			}},
			want: "10.0.0.9:8080",
		},
		{
			name: "Windows per scheme",
			snap: sysproxy.Snapshot{Windows: &sysproxy.WindowsSettings{
				ProxyEnable: sysproxy.DWORD{Present: true, Number: 1},
				ProxyServer: sysproxy.StringValue{Present: true, Text: "http=10.0.0.9:8080;https=10.0.0.9:8443"},
			}},
			want: "10.0.0.9:8443",
		},
		{
			name: "Windows switched off",
			snap: sysproxy.Snapshot{Windows: &sysproxy.WindowsSettings{
				ProxyEnable: sysproxy.DWORD{Present: true, Number: 0},
				ProxyServer: sysproxy.StringValue{Present: true, Text: "10.0.0.9:8080"},
			}},
			want: "",
		},
		{
			name: "GNOME manual",
			snap: sysproxy.Snapshot{GNOME: &sysproxy.GNOMESettings{
				Mode: "manual",
				HTTP: sysproxy.GNOMEProxy{Host: "10.0.0.9", Port: 8080},
			}},
			want: "10.0.0.9:8080",
		},
		{
			name: "GNOME automatic",
			snap: sysproxy.Snapshot{GNOME: &sysproxy.GNOMESettings{Mode: "auto"}},
			want: "",
		},
		{name: "nothing at all", snap: sysproxy.Snapshot{}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := snapshotProxy(tt.snap); got != tt.want {
				t.Errorf("snapshotProxy = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseUpstream(t *testing.T) {
	good := []string{"http://10.0.0.2:3128", "socks5://proxy.local:1080", "http://user:secret@10.0.0.2:3128"}
	for _, text := range good {
		if _, err := parseUpstream(text); err != nil {
			t.Errorf("parseUpstream(%q): %v", text, err)
		}
	}
	bad := []string{"", "10.0.0.2:3128", "ftp://10.0.0.2:21", "http://10.0.0.2", "http://10.0.0.2:0", "http://:3128"}
	for _, text := range bad {
		if _, err := parseUpstream(text); !errors.Is(err, ErrBadConfig) {
			t.Errorf("parseUpstream(%q) = %v, want ErrBadConfig", text, err)
		}
	}
}

func TestHTTPUpstreamDialerUsesCredentials(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "backend")
	}))
	defer backend.Close()
	addr, seen := startCONNECTProxy(t, backend.Listener.Addr().String())
	via, err := upstreamFor("http://user:secret@" + addr)
	if err != nil {
		t.Fatalf("upstreamFor: %v", err)
	}
	if via.proxy == nil {
		t.Error("an http upstream is not given the proxy for plain requests")
	}
	body := dialForBody(t, via.dial, "example.test:80")
	if body != "backend" {
		t.Errorf("body %q, want backend", body)
	}
	want := "CONNECT example.test:80 Basic " + base64.StdEncoding.EncodeToString([]byte("user:secret"))
	if got := <-seen; got != want {
		t.Errorf("the upstream proxy saw %q, want %q", got, want)
	}
}

func TestSOCKS5UpstreamDialerUsesCredentials(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "backend")
	}))
	defer backend.Close()
	addr, seen := startSOCKS5(t, backend.Listener.Addr().String(), "user", "secret")
	via, err := upstreamFor("socks5://user:secret@" + addr)
	if err != nil {
		t.Fatalf("upstreamFor: %v", err)
	}
	if via.proxy != nil {
		t.Error("a socks5 upstream is no HTTP proxy for plain requests")
	}
	if body := dialForBody(t, via.dial, "example.test:80"); body != "backend" {
		t.Errorf("body %q, want backend", body)
	}
	if got := <-seen; got != "example.test:80" {
		t.Errorf("the SOCKS5 proxy was asked for %q, want example.test:80", got)
	}
}

// dialForBody sends one plain HTTP request over a connection a dialer
// opens, and reads the body of the answer.
func dialForBody(t *testing.T, dial dialFunc, target string) string {
	t.Helper()
	conn, err := dial(context.Background(), "tcp", target)
	if err != nil {
		t.Fatalf("dial %s: %v", target, err)
	}
	defer func() { _ = conn.Close() }()
	request := "GET / HTTP/1.1\r\nHost: backend\r\nConnection: close\r\n\r\n"
	if _, err := io.WriteString(conn, request); err != nil {
		t.Fatalf("write: %v", err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: http.MethodGet})
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(body)
}

func TestCaptureSendsThroughTheUpstreamProxy(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "via upstream")
	}))
	defer backend.Close()
	upstream, seen := startCONNECTProxy(t, backend.Listener.Addr().String())

	tl, _ := testTool(t)
	cfg := tool.CaptureConfig{Mode: tool.ModeProxy, ProxyPort: freePort(t), Upstream: "http://" + upstream}
	if err := tl.SetConfig(cfg); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if err := tl.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = tl.Stop() })

	body := getThroughCapture(t, cfg.ProxyPort, "http://traffic.test/hello")
	if body != "via upstream" {
		t.Errorf("body %q, want the answer of the server behind the upstream proxy", body)
	}
	if got := <-seen; !strings.HasPrefix(got, "GET traffic.test") {
		t.Errorf("the upstream proxy saw %q, want a plain GET in absolute form, not a CONNECT", got)
	}
}

// getThroughCapture sends one plain HTTP request through a capture proxy
// and returns the body it got.
func getThroughCapture(t *testing.T, port int, target string) string {
	t.Helper()
	proxyURL := &url.URL{Scheme: "http", Host: net.JoinHostPort("127.0.0.1", strconv.Itoa(port))}
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
	if err != nil {
		t.Fatalf("request %s: %v", target, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", target, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(body)
}

// TestUpstreamErrorsHideCredentials: an error reaches the log and the UI,
// so the password in the URL must never be quoted.
func TestUpstreamErrorsHideCredentials(t *testing.T) {
	for _, text := range []string{
		"ftp://user:s3cret@10.0.0.2:21",
		"http://user:s3cret@10.0.0.2",
		"http://user:s3cret@10.0.0.2:0",
	} {
		_, err := parseUpstream(text)
		if err == nil {
			t.Fatalf("parseUpstream(%q): no error", text)
		}
		if strings.Contains(err.Error(), "s3cret") {
			t.Errorf("parseUpstream(%q) quotes the password: %v", text, err)
		}
		if _, err := upstreamFor(text); err == nil || strings.Contains(err.Error(), "s3cret") {
			t.Errorf("upstreamFor(%q) = %v, want an error without the password", text, err)
		}
	}
}

// TestSOCKS5HIsSOCKS5: socks5h names the scheme that resolves the name on
// the proxy side, which is what the capture does anyway.
func TestSOCKS5HIsSOCKS5(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "backend")
	}))
	defer backend.Close()
	addr, seen := startSOCKS5(t, backend.Listener.Addr().String(), "user", "secret")
	via, err := upstreamFor("socks5h://user:secret@" + addr)
	if err != nil {
		t.Fatalf("upstreamFor: %v", err)
	}
	if body := dialForBody(t, via.dial, "example.test:80"); body != "backend" {
		t.Errorf("body %q, want backend", body)
	}
	if got := <-seen; got != "example.test:80" {
		t.Errorf("the SOCKS5 proxy was asked for %q, want example.test:80", got)
	}
}
