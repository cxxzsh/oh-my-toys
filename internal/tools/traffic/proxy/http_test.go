package proxy

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/andybalholm/brotli"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/tools/traffic/rules"
)

func TestProxyForwardsHTTP(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Answer", "yes")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, "hello proxy")
	}))
	t.Cleanup(upstream.Close)
	recorded := newTestRecorder()
	srv := startProxy(t, Options{Log: recorded})
	client := clientThrough(t, srv, nil, false)

	target := upstream.URL + "/one?q=two"
	status, header, body, err := get(t, client, target)
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || string(body) != "hello proxy" {
		t.Fatalf("client saw %d %q", status, body)
	}
	if header.Get("X-Answer") != "yes" {
		t.Errorf("client header X-Answer = %q", header.Get("X-Answer"))
	}

	flow := recorded.wait(t, func(flow tool.Flow) bool { return flow.State == tool.FlowDone })
	checks := []struct {
		name string
		got  any
		want any
	}{
		{"method", flow.Method, http.MethodGet},
		{"url", flow.URL, target},
		{"host", flow.Host, strings.TrimPrefix(upstream.URL, "http://")},
		{"path", flow.Path, "/one?q=two"},
		{"proto", flow.Proto, protoHTTP11},
		{"status", flow.Status, http.StatusOK},
		{"type", flow.Type, "text/plain"},
		{"size", flow.Size, int64(len("hello proxy"))},
		{"decrypted", flow.Decrypted, false},
		{"tls version", flow.TLSVersion, ""},
	}
	for _, check := range checks {
		if check.got != check.want {
			t.Errorf("%s = %#v, want %#v", check.name, check.got, check.want)
		}
	}
	if flow.Duration <= 0 {
		t.Error("duration is not positive")
	}
	if flow.ServerAddr == "" {
		t.Error("server address is empty")
	}
	if got := headerValue(flow.Request.Headers, "User-Agent"); got == "" {
		t.Error("request headers do not record User-Agent")
	}
	if got := headerValue(flow.Request.Headers, "Host"); got != strings.TrimPrefix(upstream.URL, "http://") {
		t.Errorf("recorded Host header = %q", got)
	}
	if got := headerValue(flow.Response.Headers, "X-Answer"); got != "yes" {
		t.Errorf("recorded response X-Answer = %q", got)
	}
	if string(flow.Response.Body.Data) != "hello proxy" {
		t.Errorf("recorded response body = %q", flow.Response.Body.Data)
	}
}

func TestProxyDecodesGzipForTheRecordOnly(t *testing.T) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, _ = writer.Write([]byte("hello gzip"))
	_ = writer.Close()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write(compressed.Bytes())
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

	_, _, body, err := get(t, client, upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, compressed.Bytes()) {
		t.Fatal("the client body differs from the gzip bytes the server sent")
	}
	flow := recorded.wait(t, func(flow tool.Flow) bool { return flow.State == tool.FlowDone })
	if string(flow.Response.Body.Data) != "hello gzip" {
		t.Errorf("recorded body = %q, want the decoded text", flow.Response.Body.Data)
	}
	if flow.Response.Body.Size != int64(compressed.Len()) {
		t.Errorf("recorded size = %d, want the wire size %d", flow.Response.Body.Size, compressed.Len())
	}
}

func TestProxyBlockClosesTheConnection(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(upstream.Close)
	recorded := newTestRecorder()
	srv := startProxy(t, Options{
		Log:   recorded,
		Rules: &testRuler{verdict: rules.Verdict{Block: true, Hits: []string{"ads"}}},
	})
	client := clientThrough(t, srv, nil, false)

	if _, _, _, err := get(t, client, upstream.URL); err == nil {
		t.Fatal("a blocked request got an answer")
	}
	flow := recorded.wait(t, func(flow tool.Flow) bool { return flow.State == tool.FlowBlocked })
	if !flow.Ruled || flow.RuleHits[0] != "ads" {
		t.Errorf("blocked flow ruled %v with hits %v", flow.Ruled, flow.RuleHits)
	}
}

func TestProxyMockAnswersWithoutTheServer(t *testing.T) {
	var served bool
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { served = true }))
	t.Cleanup(upstream.Close)
	recorded := newTestRecorder()
	body := "mocked"
	srv := startProxy(t, Options{
		Log: recorded,
		Rules: &testRuler{verdict: rules.Verdict{
			Mock: &http.Response{
				StatusCode:    http.StatusNonAuthoritativeInfo,
				Status:        "203 Non-Authoritative Information",
				Proto:         protoHTTP11,
				ProtoMajor:    1,
				ProtoMinor:    1,
				Header:        http.Header{"X-Mock": []string{"yes"}},
				Body:          io.NopCloser(strings.NewReader(body)),
				ContentLength: int64(len(body)),
			},
			Hits: []string{"stub"},
		}},
	})
	client := clientThrough(t, srv, nil, false)

	status, header, answer, err := get(t, client, upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusNonAuthoritativeInfo || string(answer) != body || header.Get("X-Mock") != "yes" {
		t.Fatalf("client saw %d %q %v", status, answer, header)
	}
	if served {
		t.Error("the mocked request reached the server")
	}
	flow := recorded.wait(t, func(flow tool.Flow) bool { return flow.State == tool.FlowDone })
	if !flow.Ruled || flow.RuleHits[0] != "stub" {
		t.Errorf("mock flow ruled %v with hits %v", flow.Ruled, flow.RuleHits)
	}
	if string(flow.Response.Body.Data) != body {
		t.Errorf("recorded mock body = %q", flow.Response.Body.Data)
	}
}

func TestProxyDelayHoldsTheAnswer(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "late")
	}))
	t.Cleanup(upstream.Close)
	srv := startProxy(t, Options{Rules: &testRuler{verdict: rules.Verdict{Delay: 150 * time.Millisecond}}})
	client := clientThrough(t, srv, nil, false)

	start := time.Now()
	status, _, body, err := get(t, client, upstream.URL)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || string(body) != "late" {
		t.Fatalf("client saw %d %q", status, body)
	}
	if elapsed < 120*time.Millisecond {
		t.Errorf("answer came after %v, want the 150ms delay", elapsed)
	}
}

func TestProxyAppliesResponseRewrites(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "original")
	}))
	t.Cleanup(upstream.Close)
	recorded := newTestRecorder()
	rewrite := "rewritten"
	ruler := &testRuler{hits: []string{"teapot"}}
	ruler.response = func(_ *http.Request, resp *http.Response) {
		resp.StatusCode = http.StatusTeapot
		resp.Status = "418 I'm a teapot"
		resp.Header.Set("Content-Length", strconv.Itoa(len(rewrite)))
		resp.Header.Set("X-Rewritten", "yes")
		resp.ContentLength = int64(len(rewrite))
		resp.Body = io.NopCloser(strings.NewReader(rewrite))
	}
	srv := startProxy(t, Options{Log: recorded, Rules: ruler})
	client := clientThrough(t, srv, nil, false)

	status, header, body, err := get(t, client, upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusTeapot || string(body) != rewrite || header.Get("X-Rewritten") != "yes" {
		t.Fatalf("client saw %d %q %v", status, body, header)
	}
	flow := recorded.wait(t, func(flow tool.Flow) bool { return flow.State == tool.FlowDone })
	if !flow.Ruled || flow.RuleHits[0] != "teapot" || flow.Status != http.StatusTeapot {
		t.Errorf("rewritten flow: ruled %v hits %v status %d", flow.Ruled, flow.RuleHits, flow.Status)
	}
	if string(flow.Response.Body.Data) != rewrite {
		t.Errorf("recorded body = %q", flow.Response.Body.Data)
	}
}

func TestProxyRefusesToLoop(t *testing.T) {
	recorded := newTestRecorder()
	srv := startProxy(t, Options{Log: recorded})
	client := clientThrough(t, srv, nil, false)

	status, _, _, err := get(t, client, "http://"+srv.Addr()+"/loop")
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusLoopDetected {
		t.Fatalf("status = %d, want 508", status)
	}
	flow := recorded.wait(t, func(flow tool.Flow) bool { return flow.State == tool.FlowFailed })
	if !strings.Contains(flow.Info, "targets the proxy itself") {
		t.Errorf("flow info = %q", flow.Info)
	}
}

// headerValue is the first value of a recorded header, or "".
func headerValue(headers []tool.Header, name string) string {
	for _, header := range headers {
		if strings.EqualFold(header.Name, name) {
			return header.Value
		}
	}
	return ""
}

func TestProxyDecodesBrotliForTheRecordOnly(t *testing.T) {
	var compressed bytes.Buffer
	writer := brotli.NewWriter(&compressed)
	if _, err := writer.Write([]byte("hello brotli")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Encoding", "br")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write(compressed.Bytes())
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

	_, _, body, err := get(t, client, upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, compressed.Bytes()) {
		t.Fatal("the client body differs from the brotli bytes the server sent")
	}
	flow := recorded.wait(t, func(flow tool.Flow) bool { return flow.State == tool.FlowDone })
	if string(flow.Response.Body.Data) != "hello brotli" {
		t.Errorf("recorded body = %q, want the decoded text", flow.Response.Body.Data)
	}
	if flow.Response.Body.Size != int64(compressed.Len()) {
		t.Errorf("recorded size = %d, want the wire size %d", flow.Response.Body.Size, compressed.Len())
	}
}
