package proxy

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/tools/traffic/ca"
	"github.com/cxxzsh/oh-my-toys/internal/tools/traffic/rules"
)

// testRoot is the CA the fake upstream servers use. Tests hand it to the
// proxy through Options.RootCAs, so verification runs against it in this
// process only, however often the suite runs.
var testRoot *testPKI

func TestMain(m *testing.M) {
	pki, err := newPKI()
	if err != nil {
		fmt.Fprintln(os.Stderr, "make the test certificate authority:", err)
		os.Exit(1)
	}
	testRoot = pki
	os.Exit(m.Run())
}

// testPKI is a CA and a server certificate for 127.0.0.1 and localhost.
type testPKI struct {
	caPEM []byte
	leaf  tls.Certificate
}

// newPKI makes a fresh certificate authority and one server certificate.
func newPKI() (*testPKI, error) {
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "traffic test root"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, err
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		return nil, err
	}
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		return nil, err
	}
	leafCert, err := x509.ParseCertificate(leafDER)
	if err != nil {
		return nil, err
	}
	return &testPKI{
		caPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}),
		leaf:  tls.Certificate{Certificate: [][]byte{leafDER}, PrivateKey: leafKey, Leaf: leafCert},
	}, nil
}

// roots is a pool holding only the test root.
func (p *testPKI) roots() *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(p.caPEM)
	return pool
}

// proxyCA makes the root certificate the proxy uses to open HTTPS.
func proxyCA(t *testing.T) *ca.Authority {
	t.Helper()
	authority, err := ca.Create(filepath.Join(t.TempDir(), "ca"))
	if err != nil {
		t.Fatal(err)
	}
	return authority
}

// testRecorder keeps the flows the proxy wrote.
type testRecorder struct {
	mu    sync.Mutex
	flows map[uint64]tool.Flow
	order []uint64
	next  uint64
}

func newTestRecorder() *testRecorder {
	return &testRecorder{flows: make(map[uint64]tool.Flow)}
}

// Put records a flow the way flowlog.Log does.
func (r *testRecorder) Put(flow tool.Flow) uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	if flow.ID == 0 {
		r.next++
		flow.ID = r.next
		r.order = append(r.order, flow.ID)
	}
	r.flows[flow.ID] = flow
	return flow.ID
}

// flowsNow is every flow the recorder holds, oldest first.
func (r *testRecorder) flowsNow() []tool.Flow {
	r.mu.Lock()
	defer r.mu.Unlock()
	flows := make([]tool.Flow, 0, len(r.order))
	for _, id := range r.order {
		if flow, ok := r.flows[id]; ok {
			flows = append(flows, flow)
		}
	}
	return flows
}

// wait waits for a flow that matches, and fails the test when none does.
func (r *testRecorder) wait(t *testing.T, match func(tool.Flow) bool) tool.Flow {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, flow := range r.flowsNow() {
			if match(flow) {
				return flow
			}
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("no flow matched; recorded %d flows: %v", len(r.flowsNow()), summaries(r.flowsNow()))
	return tool.Flow{}
}

// summaries is a short description of the flows for a failure message.
func summaries(flows []tool.Flow) []string {
	lines := make([]string, 0, len(flows))
	for _, flow := range flows {
		lines = append(lines, fmt.Sprintf("%d %s %s %s %d state %d %q", flow.ID, flow.Method, flow.URL, flow.Proto, flow.Status, flow.State, flow.Info))
	}
	return lines
}

// testRuler is a fake rule engine the tests steer.
type testRuler struct {
	mu       sync.Mutex
	verdict  rules.Verdict
	response func(*http.Request, *http.Response)
	hits     []string
}

// Request returns the verdict the test set up.
func (r *testRuler) Request(*http.Request) (rules.Verdict, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.verdict, nil
}

// Response applies the rewrite the test set up.
func (r *testRuler) Response(req *http.Request, resp *http.Response) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.response != nil {
		r.response(req, resp)
	}
	return r.hits, nil
}

// startProxy starts a server the test stops when it ends.
func startProxy(t *testing.T, o Options) *Server {
	t.Helper()
	if o.Addr == "" {
		o.Addr = "127.0.0.1:0"
	}
	if o.RootCAs == nil {
		o.RootCAs = testRoot.roots()
	}
	srv, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := srv.Stop(); err != nil {
			t.Errorf("stop: %v", err)
		}
	})
	return srv
}

// startProxyTuned starts a server after letting the test shorten its
// timeouts, which must be set before Start.
func startProxyTuned(t *testing.T, o Options, tune func(*Server)) *Server {
	t.Helper()
	if o.Addr == "" {
		o.Addr = "127.0.0.1:0"
	}
	if o.RootCAs == nil {
		o.RootCAs = testRoot.roots()
	}
	srv, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	tune(srv)
	if err := srv.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := srv.Stop(); err != nil {
			t.Errorf("stop: %v", err)
		}
	})
	return srv
}

// proxyURL is the URL of a running proxy.
func proxyURL(t *testing.T, srv *Server) *url.URL {
	t.Helper()
	parsed, err := url.Parse("http://" + srv.Addr())
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

// clientThrough is an HTTP client that sends everything through srv.
func clientThrough(t *testing.T, srv *Server, tlsConfig *tls.Config, http2 bool) *http.Client {
	t.Helper()
	transport := &http.Transport{
		Proxy:             http.ProxyURL(proxyURL(t, srv)),
		DisableKeepAlives: true,
		ForceAttemptHTTP2: http2,
		TLSClientConfig:   tlsConfig,
	}
	t.Cleanup(transport.CloseIdleConnections)
	return &http.Client{Transport: transport, Timeout: 10 * time.Second}
}

// newUpstream starts an HTTPS server with a certificate from the test root.
func newUpstream(t *testing.T, handler http.Handler, http2 bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(handler)
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{testRoot.leaf}}
	srv.EnableHTTP2 = http2
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

// get reads a whole response body and closes it.
func get(t *testing.T, client *http.Client, target string) (int, http.Header, []byte, error) {
	t.Helper()
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
	if err != nil {
		return 0, nil, nil, err
	}
	resp, err := client.Do(request)
	if err != nil {
		return 0, nil, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, body, err
}
