package proxy

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// fixedProgram is a hook that names every client connection the same way.
func fixedProgram(name string) func(netip.AddrPort) tool.Program {
	return func(netip.AddrPort) tool.Program {
		return tool.Program{PID: 42, Name: name, Path: "/bin/" + name}
	}
}

func TestFlowsCarryTheProgramOfTheConnection(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(greet))
	t.Cleanup(upstream.Close)
	recorded := newTestRecorder()
	srv := startProxy(t, Options{Log: recorded, Program: fixedProgram("curl")})
	client := clientThrough(t, srv, nil, false)

	if _, _, _, err := get(t, client, upstream.URL); err != nil {
		t.Fatal(err)
	}
	flow := recorded.wait(t, func(flow tool.Flow) bool { return flow.State == tool.FlowDone })
	if flow.Process.Name != "curl" || flow.Process.PID != 42 {
		t.Errorf("flow process = %+v, want curl pid 42", flow.Process)
	}
}

func TestScopeRecordsTheListedProgramOnly(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(greet))
	t.Cleanup(upstream.Close)
	recorded := newTestRecorder()
	srv := startProxy(t, Options{
		Log:     recorded,
		Program: fixedProgram("Wanted"),
		Scope:   []string{"  wanted "},
	})
	client := clientThrough(t, srv, nil, false)

	status, _, body, err := get(t, client, upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || string(body) != "hello through tls" {
		t.Fatalf("client saw %d %q", status, body)
	}
	flow := recorded.wait(t, func(flow tool.Flow) bool { return flow.State == tool.FlowDone })
	if flow.Process.Name != "Wanted" {
		t.Errorf("flow process = %q, want Wanted", flow.Process.Name)
	}
}

func TestScopeForwardsOtherProgramsUnrecorded(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(greet))
	t.Cleanup(upstream.Close)
	recorded := newTestRecorder()
	srv := startProxy(t, Options{
		Log:     recorded,
		Program: fixedProgram("other"),
		Scope:   []string{"wanted"},
	})
	client := clientThrough(t, srv, nil, false)

	status, _, body, err := get(t, client, upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || string(body) != "hello through tls" {
		t.Fatalf("client saw %d %q, want the upstream answer", status, body)
	}
	if flows := recorded.flowsNow(); len(flows) != 0 {
		t.Errorf("recorded %d flows for an out-of-scope program", len(flows))
	}
}

func TestScopeWithAnUnknownProgramRecordsNothing(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(greet))
	t.Cleanup(upstream.Close)
	recorded := newTestRecorder()
	srv := startProxy(t, Options{Log: recorded, Scope: []string{"wanted"}})
	client := clientThrough(t, srv, nil, false)

	if _, _, _, err := get(t, client, upstream.URL); err != nil {
		t.Fatal(err)
	}
	if flows := recorded.flowsNow(); len(flows) != 0 {
		t.Errorf("recorded %d flows with an unknown program", len(flows))
	}
}

func TestScopeTunnelsAnOutOfScopeConnectBlindly(t *testing.T) {
	upstream := newUpstream(t, http.HandlerFunc(greet), false)
	recorded := newTestRecorder()
	srv := startProxy(t, Options{
		Log:       recorded,
		Authority: proxyCA(t),
		Decrypt:   true,
		Program:   fixedProgram("other"),
		Scope:     []string{"wanted"},
	})
	client := clientThrough(t, srv, &tls.Config{
		RootCAs:    testRoot.roots(),
		NextProtos: []string{"http/1.1"},
	}, false)

	// The client trusts only the upstream's root, so a decrypting proxy
	// would fail the handshake; getting the body proves the bytes went
	// through untouched.
	_, _, body, err := get(t, client, upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "hello through tls" {
		t.Fatalf("client body = %q", body)
	}
	if flows := recorded.flowsNow(); len(flows) != 0 {
		t.Errorf("recorded %d flows for an out-of-scope tunnel", len(flows))
	}
}

func TestProgramLookupNeverHoldsTheConnection(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(greet))
	t.Cleanup(upstream.Close)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	recorded := newTestRecorder()
	srv := startProxy(t, Options{
		Log: recorded,
		Program: func(netip.AddrPort) tool.Program {
			<-release
			return tool.Program{Name: "late"}
		},
	})
	client := clientThrough(t, srv, nil, false)

	started := time.Now()
	if _, _, _, err := get(t, client, upstream.URL); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Errorf("request took %v with a stuck program lookup", elapsed)
	}
	flow := recorded.wait(t, func(flow tool.Flow) bool { return flow.State == tool.FlowDone })
	if flow.Process != (tool.Program{}) {
		t.Errorf("flow process = %+v, want none after the lookup timed out", flow.Process)
	}
}

func TestNormalizeScopeFoldsCaseAndDropsBlanks(t *testing.T) {
	got := normalizeScope([]string{" Chrome ", "", "  ", "Safari"})
	want := []string{"chrome", "safari"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("normalizeScope = %q, want %q", got, want)
	}
}
