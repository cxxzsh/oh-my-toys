package traffic

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/tools/traffic/ca"
)

// wantBody is what the upstream of the end-to-end test answers.
const wantBody = `{"ok":true}`

// harDoc is as much of HAR 1.2 as the export test reads back.
type harDoc struct {
	Log struct {
		Version string `json:"version"`
		Entries []struct {
			Request struct {
				Method string `json:"method"`
				URL    string `json:"url"`
			} `json:"request"`
			Response struct {
				Status  int `json:"status"`
				Content struct {
					MimeType string `json:"mimeType"`
					Text     string `json:"text"`
				} `json:"content"`
			} `json:"response"`
		} `json:"entries"`
	} `json:"log"`
}

// TestDecryptsReplaysAndExports walks the whole thing: a client that trusts
// the tool's root sends one HTTPS request through the capture, the flow is
// recorded decrypted, sent again, and written to a HAR file.
func TestDecryptsReplaysAndExports(t *testing.T) {
	tl, cfg, upstream, root := startDecryptingCapture(t)
	client := captureClient(t, root, cfg.ProxyPort)
	wantURL := upstream.URL + "/api?x=1"
	if status, body := get(t, client, wantURL); status != http.StatusOK || body != wantBody {
		t.Fatalf("GET through the capture: %d %q", status, body)
	}

	rows := waitRows(t, tl.Log(), tool.SourceProxy, 1)
	flow := flowOf(t, tl, rows[0].ID)
	checkDecrypted(t, flow, wantURL)

	replayed := replay(t, tl, flow.ID)
	if second := flowOf(t, tl, replayed); second.ReplayOf != flow.ID {
		t.Errorf("ReplayOf = %d, want %d", second.ReplayOf, flow.ID)
	}

	checkHAR(t, exportHAR(t, tl, nil), 2)
	checkHAR(t, exportHAR(t, tl, []uint64{replayed}), 1)
}

// startDecryptingCapture starts a capture proxy that opens HTTPS, its
// upstream signed by the tool's own root.
func startDecryptingCapture(t *testing.T) (*Tool, tool.CaptureConfig, *httptest.Server, *ca.Authority) {
	t.Helper()
	dir := t.TempDir()
	root, err := ca.Create(filepath.Join(dir, trafficDir, caFolder))
	if err != nil {
		t.Fatalf("ca.Create: %v", err)
	}
	// The proxy verifies the server it reaches against the roots it is
	// given, and the upstream below is signed by the tool's own root, so
	// the test hands the proxy a pool instead of touching a trust store.
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(root.CertPEM()) {
		t.Fatal("the root certificate does not parse")
	}

	upstream := startUpstream(t, root)
	tl := newTool(dir, (&fakeSystem{now: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}).system())
	tl.rootCAs = pool
	cfg := tool.CaptureConfig{Mode: tool.ModeProxy, ProxyPort: freePort(t), Decrypt: true}
	if err := tl.SetConfig(cfg); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if err := tl.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = tl.Stop() })
	return tl, cfg, upstream, root
}

// flowOf reads a recorded flow.
func flowOf(t *testing.T, tl *Tool, id uint64) tool.Flow {
	t.Helper()
	flow, ok := tl.Log().Get(id)
	if !ok {
		t.Fatalf("flow %d is not recorded", id)
	}
	return flow
}

// checkDecrypted checks what the capture recorded of one request.
func checkDecrypted(t *testing.T, flow tool.Flow, wantURL string) {
	t.Helper()
	if !flow.Decrypted {
		t.Error("the flow is not marked as decrypted")
	}
	if flow.URL != wantURL {
		t.Errorf("URL %q, want %q", flow.URL, wantURL)
	}
	if flow.Response.Body.Type != "application/json" {
		t.Errorf("body type %q, want application/json", flow.Response.Body.Type)
	}
	if string(flow.Response.Body.Data) != wantBody {
		t.Errorf("body %q, want %q", flow.Response.Body.Data, wantBody)
	}
}

// replay sends a flow again and returns the ID of the new one.
func replay(t *testing.T, tl *Tool, id uint64) uint64 {
	t.Helper()
	replayed, err := tl.Replay(context.Background(), id)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if replayed == id {
		t.Error("the replay reused the ID of the flow it sent again")
	}
	return replayed
}

// checkHAR checks the file the export wrote, which the caller has read
// back already.
func checkHAR(t *testing.T, doc harDoc, least int) {
	t.Helper()
	if doc.Log.Version != "1.2" {
		t.Errorf("HAR version %q, want 1.2", doc.Log.Version)
	}
	if len(doc.Log.Entries) < least {
		t.Fatalf("%d entries, want at least %d", len(doc.Log.Entries), least)
	}
	first := doc.Log.Entries[0]
	if first.Request.Method != http.MethodGet || first.Response.Status != http.StatusOK {
		t.Errorf("first entry %s %d, want GET 200", first.Request.Method, first.Response.Status)
	}
	if first.Response.Content.Text != wantBody {
		t.Errorf("first entry body %q, want the decoded body", first.Response.Content.Text)
	}
}

// startUpstream runs a TLS server that answers one fixed body, signed by
// the tool's root so that the capture can verify it.
func startUpstream(t *testing.T, root *ca.Authority) *httptest.Server {
	t.Helper()
	leaf, err := root.Leaf("127.0.0.1")
	if err != nil {
		t.Fatalf("leaf certificate: %v", err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, wantBody)
	}))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{*leaf}, MinVersion: tls.VersionTLS12}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

// captureClient is a client that sends its requests through the capture
// and trusts the tool's root, as a person's machine does once the root is
// installed.
func captureClient(t *testing.T, root *ca.Authority, port int) *http.Client {
	t.Helper()
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(root.CertPEM()) {
		t.Fatal("the root certificate does not parse")
	}
	proxyURL := &url.URL{Scheme: "http", Host: "127.0.0.1:" + strconv.Itoa(port)}
	return &http.Client{Transport: &http.Transport{
		Proxy:           http.ProxyURL(proxyURL),
		TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
	}}
}

// get sends one request and returns the status and the body of the answer.
func get(t *testing.T, client *http.Client, target string) (int, string) {
	t.Helper()
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
	return resp.StatusCode, string(body)
}

// exportHAR writes the flows of ids, or every flow when ids is empty, and
// reads the file back.
func exportHAR(t *testing.T, tl *Tool, ids []uint64) harDoc {
	t.Helper()
	var buf bytes.Buffer
	if err := tl.ExportHAR(&buf, ids); err != nil {
		t.Fatalf("ExportHAR: %v", err)
	}
	var doc harDoc
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("the HAR file does not parse: %v\n%s", err, buf.String())
	}
	return doc
}
