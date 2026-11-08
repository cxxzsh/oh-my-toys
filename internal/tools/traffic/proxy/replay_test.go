package proxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// echo answers with the method, a header and the body it got.
func echo(w http.ResponseWriter, req *http.Request) {
	body, _ := io.ReadAll(req.Body)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, req.Method+" "+req.Header.Get("X-Replay")+" "+string(body))
}

// recordPost sends one POST through the proxy and returns its flow.
func recordPost(t *testing.T, upstream *httptest.Server, client *http.Client, recorded *testRecorder) tool.Flow {
	t.Helper()
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, upstream.URL+"/echo", strings.NewReader("payload"))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Replay", "first")
	resp, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	flow := recorded.wait(t, func(flow tool.Flow) bool { return flow.State == tool.FlowDone })
	if string(flow.Request.Body.Data) != "payload" {
		t.Fatalf("recorded request body = %q", flow.Request.Body.Data)
	}
	return flow
}

func TestRequestRebuildsAFlow(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(echo))
	t.Cleanup(upstream.Close)
	recorded := newTestRecorder()
	srv := startProxy(t, Options{Log: recorded})
	flow := recordPost(t, upstream, clientThrough(t, srv, nil, false), recorded)

	rebuilt, err := Request(flow)
	if err != nil {
		t.Fatal(err)
	}
	if rebuilt.Method != http.MethodPost || rebuilt.URL.String() != upstream.URL+"/echo" {
		t.Errorf("rebuilt request is %s %s", rebuilt.Method, rebuilt.URL)
	}
	if got := rebuilt.Header.Get("X-Replay"); got != "first" {
		t.Errorf("rebuilt header X-Replay = %q", got)
	}
	if rebuilt.ContentLength != int64(len("payload")) {
		t.Errorf("rebuilt content length = %d", rebuilt.ContentLength)
	}
	body, err := io.ReadAll(rebuilt.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "payload" {
		t.Errorf("rebuilt body = %q", body)
	}
}

func TestDoSendsAFlowAgain(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(echo))
	t.Cleanup(upstream.Close)
	recorded := newTestRecorder()
	srv := startProxy(t, Options{Log: recorded})
	flow := recordPost(t, upstream, clientThrough(t, srv, nil, false), recorded)

	again, err := Request(flow)
	if err != nil {
		t.Fatal(err)
	}
	id, err := srv.Do(context.Background(), again, flow.ID)
	if err != nil {
		t.Fatal(err)
	}
	replayed := recorded.wait(t, func(flow tool.Flow) bool { return flow.ID == id && flow.State == tool.FlowDone })
	if replayed.ReplayOf != flow.ID {
		t.Errorf("replayed flow records ReplayOf %d, want %d", replayed.ReplayOf, flow.ID)
	}
	if string(replayed.Request.Body.Data) != "payload" {
		t.Errorf("replayed request body = %q", replayed.Request.Body.Data)
	}
	if string(replayed.Response.Body.Data) != "POST first payload" {
		t.Errorf("replayed response body = %q", replayed.Response.Body.Data)
	}
}

func TestRequestRefusesACutBody(t *testing.T) {
	flow := tool.Flow{
		FlowRow: tool.FlowRow{ID: 7, Method: http.MethodPost},
		URL:     "http://example.test/upload",
	}
	flow.Request.Body.Truncated = true
	_, err := Request(flow)
	if err == nil || !strings.Contains(err.Error(), "cut") {
		t.Fatalf("Request of a cut body = %v, want an error saying it was cut", err)
	}
}

func TestRequestKeepsHeaderNamesAndDropsHopByHop(t *testing.T) {
	flow := tool.Flow{
		FlowRow: tool.FlowRow{ID: 9, Method: http.MethodGet, Host: "example.test"},
		URL:     "http://example.test/a",
		Request: tool.HTTPMessage{Headers: []tool.Header{
			{Name: "Connection", Value: "close"},
			{Name: "Content-Length", Value: "3"},
			{Name: "X-Keep", Value: "yes"},
			{Name: "x-lower", Value: "case"},
		}},
	}
	request, err := Request(flow)
	if err != nil {
		t.Fatal(err)
	}
	if request.Header.Get("Connection") != "" || request.Header.Get("Content-Length") != "" {
		t.Errorf("hop-by-hop headers survived: %v", request.Header)
	}
	if request.Header["X-Keep"][0] != "yes" {
		t.Errorf("X-Keep = %v", request.Header["X-Keep"])
	}
	lower := "x-lower"
	if request.Header[lower][0] != "case" {
		t.Errorf("lower-case header name was not kept: %v", request.Header)
	}
}

func TestRequestNeedsAnAbsoluteURL(t *testing.T) {
	if _, err := Request(tool.Flow{URL: "/relative"}); err == nil {
		t.Fatal("Request accepted a relative URL")
	}
}
