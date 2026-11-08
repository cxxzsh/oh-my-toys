package har

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// The document as a reader decodes it: the required fields of HAR 1.2, with
// the names the format uses.

type testDocument struct {
	Log testLog `json:"log"`
}

type testLog struct {
	Version string      `json:"version"`
	Creator testCreator `json:"creator"`
	Entries []testEntry `json:"entries"`
}

type testCreator struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type testEntry struct {
	Started  string       `json:"startedDateTime"`
	Time     float64      `json:"time"`
	Request  testRequest  `json:"request"`
	Response testResponse `json:"response"`
	Cache    any          `json:"cache"`
	Timings  testTimings  `json:"timings"`
}

type testRequest struct {
	Method      string          `json:"method"`
	URL         string          `json:"url"`
	HTTPVersion string          `json:"httpVersion"`
	Cookies     []testCookie    `json:"cookies"`
	Headers     []testHeader    `json:"headers"`
	QueryString []testParameter `json:"queryString"`
	PostData    *testPostData   `json:"postData"`
	HeadersSize int             `json:"headersSize"`
	BodySize    int64           `json:"bodySize"`
}

type testResponse struct {
	Status      int          `json:"status"`
	StatusText  string       `json:"statusText"`
	HTTPVersion string       `json:"httpVersion"`
	Cookies     []testCookie `json:"cookies"`
	Headers     []testHeader `json:"headers"`
	Content     testContent  `json:"content"`
	RedirectURL string       `json:"redirectURL"`
	HeadersSize int          `json:"headersSize"`
	BodySize    int64        `json:"bodySize"`
}

type testCookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Path     string `json:"path"`
	HTTPOnly bool   `json:"httpOnly"`
	Secure   bool   `json:"secure"`
}

type testHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type testParameter struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type testPostData struct {
	MimeType string `json:"mimeType"`
	Text     string `json:"text"`
}

type testContent struct {
	Size     int64  `json:"size"`
	MimeType string `json:"mimeType"`
	Text     string `json:"text"`
	Encoding string `json:"encoding"`
}

type testTimings struct {
	Send    float64 `json:"send"`
	Wait    float64 `json:"wait"`
	Receive float64 `json:"receive"`
}

// started is the time the sample flows started.
var started = time.Date(2026, 11, 8, 10, 30, 0, 123_000_000, time.UTC)

// sampleFlows holds one text exchange, one binary one, a packet and the two
// shapes of CONNECT tunnel.
func sampleFlows() []tool.Flow {
	json := tool.Flow{
		FlowRow: tool.FlowRow{
			ID: 7, Source: tool.SourceProxy, Started: started, Method: "POST",
			Host: "example.com:443", Path: "/search?q=go+lang&lang=en",
			Proto: "HTTP/1.1", Status: 200, Type: "application/json",
			Size: 40, Duration: 150 * time.Millisecond, State: tool.FlowDone,
		},
		URL: "https://example.com/search?q=go+lang&lang=en",
		Request: tool.HTTPMessage{
			Headers: []tool.Header{
				{Name: "Host", Value: "example.com"},
				{Name: "Cookie", Value: "a=1; b=2"},
				{Name: "Content-Type", Value: "application/json"},
			},
			Body: tool.Body{Type: "application/json", Data: []byte(`{"x":1}`), Size: 7},
		},
		Response: tool.HTTPMessage{
			Headers: []tool.Header{
				{Name: "Content-Type", Value: "application/json"},
				{Name: "Set-Cookie", Value: "sid=abc; Path=/; HttpOnly; Secure"},
			},
			Body: tool.Body{Type: "application/json", Data: []byte(`{"ok":true}`), Size: 40},
		},
	}
	binary := tool.Flow{
		FlowRow: tool.FlowRow{
			ID: 8, Source: tool.SourceProxy, Started: started.Add(time.Second),
			Method: "GET", Host: "example.com:443", Path: "/logo.png",
			Proto: "HTTP/1.1", Status: 200, Type: "image/png",
			Duration: 20 * time.Millisecond, State: tool.FlowDone,
		},
		URL:      "https://example.com/logo.png",
		Response: tool.HTTPMessage{Body: tool.Body{Type: "image/png", Data: []byte{0x89, 'P', 'N', 'G'}, Size: 100}},
	}
	packet := tool.Flow{
		FlowRow: tool.FlowRow{
			ID: 9, Source: tool.SourcePacket, Started: started, Host: "8.8.8.8:53",
			Proto: "DNS", Size: 74, Info: "DNS query example.org",
		},
		Packet: &tool.PacketDetail{Raw: []byte{1, 2, 3}},
	}
	tunnel := tool.Flow{
		FlowRow: tool.FlowRow{
			ID: 10, Source: tool.SourceProxy, Started: started, Method: "CONNECT",
			Host: "example.com:443", Proto: "tunnel", State: tool.FlowDone,
		},
		URL: "example.com:443",
	}
	opened := tool.Flow{
		FlowRow: tool.FlowRow{
			ID: 11, Source: tool.SourceProxy, Started: started, Method: "CONNECT",
			Host: "api.example.com:443", Proto: "tunnel", State: tool.FlowDone,
		},
		URL:      "api.example.com:443",
		Response: tool.HTTPMessage{Body: tool.Body{Type: "application/json", Data: []byte(`{}`)}},
	}
	return []tool.Flow{json, binary, packet, tunnel, opened}
}

func writeDocument(t *testing.T, flows []tool.Flow) testDocument {
	t.Helper()
	var buf bytes.Buffer
	if err := Write(&buf, flows); err != nil {
		t.Fatalf("write: %v", err)
	}
	var doc testDocument
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("decode %q: %v", buf.String(), err)
	}
	return doc
}

func TestWriteDocument(t *testing.T) {
	doc := writeDocument(t, sampleFlows())
	if doc.Log.Version != "1.2" {
		t.Errorf("version %q", doc.Log.Version)
	}
	if doc.Log.Creator.Name != "oh-my-toys" || doc.Log.Creator.Version == "" {
		t.Errorf("creator %+v", doc.Log.Creator)
	}
	if len(doc.Log.Entries) != 3 {
		t.Fatalf("%d entries, want 3", len(doc.Log.Entries))
	}
	if doc.Log.Entries[2].Request.Method != "CONNECT" {
		t.Errorf("the opened tunnel was left out: %+v", doc.Log.Entries[2].Request)
	}
}

func TestWriteTextExchange(t *testing.T) {
	doc := writeDocument(t, sampleFlows())
	got := doc.Log.Entries[0]
	if got.Started != "2026-11-08T10:30:00.123Z" {
		t.Errorf("started %q", got.Started)
	}
	if got.Time != 150 {
		t.Errorf("time %v", got.Time)
	}
	if got.Cache == nil {
		t.Error("no cache")
	}
	wantRequest := testRequest{
		Method: "POST", URL: "https://example.com/search?q=go+lang&lang=en",
		HTTPVersion: "HTTP/1.1",
		Cookies:     []testCookie{{Name: "a", Value: "1"}, {Name: "b", Value: "2"}},
		Headers: []testHeader{
			{Name: "Host", Value: "example.com"},
			{Name: "Cookie", Value: "a=1; b=2"},
			{Name: "Content-Type", Value: "application/json"},
		},
		QueryString: []testParameter{{Name: "q", Value: "go lang"}, {Name: "lang", Value: "en"}},
		PostData:    &testPostData{MimeType: "application/json", Text: `{"x":1}`},
		HeadersSize: -1,
		BodySize:    7,
	}
	if !equalJSON(t, got.Request, wantRequest) {
		t.Errorf("request %+v", got.Request)
	}

	if got.Response.Status != 200 || got.Response.StatusText != "OK" {
		t.Errorf("status %d %q", got.Response.Status, got.Response.StatusText)
	}
	if got.Response.HeadersSize != -1 || got.Response.BodySize != 40 {
		t.Errorf("sizes %d %d", got.Response.HeadersSize, got.Response.BodySize)
	}
	if len(got.Response.Headers) != 2 || got.Response.Headers[1].Name != "Set-Cookie" {
		t.Errorf("response headers %+v", got.Response.Headers)
	}
	if len(got.Response.Cookies) != 1 {
		t.Fatalf("response cookies %+v", got.Response.Cookies)
	}
	gotCookie := got.Response.Cookies[0]
	wantCookie := testCookie{Name: "sid", Value: "abc", Path: "/", HTTPOnly: true, Secure: true}
	if gotCookie != wantCookie {
		t.Errorf("cookie %+v", gotCookie)
	}
	wantContent := testContent{Size: 11, MimeType: "application/json", Text: `{"ok":true}`}
	if got.Response.Content != wantContent {
		t.Errorf("content %+v", got.Response.Content)
	}
	if got.Timings != (testTimings{Wait: 150}) {
		t.Errorf("timings %+v", got.Timings)
	}
}

func TestWriteBinaryContent(t *testing.T) {
	doc := writeDocument(t, sampleFlows())
	got := doc.Log.Entries[1]
	if got.Request.PostData != nil {
		t.Errorf("a request without a body has postData %+v", got.Request.PostData)
	}
	content := got.Response.Content
	if content.Encoding != "base64" || content.MimeType != "image/png" || content.Size != 4 {
		t.Fatalf("content %+v", content)
	}
	decoded, err := base64.StdEncoding.DecodeString(content.Text)
	if err != nil {
		t.Fatalf("base64 %q: %v", content.Text, err)
	}
	if !bytes.Equal(decoded, []byte{0x89, 'P', 'N', 'G'}) {
		t.Errorf("bytes %v", decoded)
	}
}

// TestWriteRequiredFields checks that every entry carries the fields a HAR
// viewer requires, not just the ones these flows fill.
func TestWriteRequiredFields(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, sampleFlows()); err != nil {
		t.Fatal(err)
	}
	var raw struct {
		Log struct {
			Entries []map[string]any `json:"entries"`
		} `json:"log"`
	}
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	required := map[string][]string{
		"":         {"startedDateTime", "time", "request", "response", "cache", "timings"},
		"request":  {"method", "url", "httpVersion", "cookies", "headers", "queryString", "headersSize", "bodySize"},
		"response": {"status", "statusText", "httpVersion", "cookies", "headers", "content", "redirectURL", "headersSize", "bodySize"},
		"timings":  {"send", "wait", "receive"},
	}
	for _, entry := range raw.Log.Entries {
		for field, names := range required {
			object := entry
			if field != "" {
				object, _ = entry[field].(map[string]any)
			}
			for _, name := range names {
				if _, ok := object[name]; !ok {
					t.Fatalf("%s lacks %q", field+" entry", name)
				}
			}
		}
	}
}

// failingWriter refuses everything after limit writes.
type failingWriter struct {
	writes int
	limit  int
	err    error
}

func (w *failingWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes > w.limit {
		return 0, w.err
	}
	return len(p), nil
}

// TestWriteReportsWriterFailure checks that entries go out as they are made
// rather than after the whole document is built.
func TestWriteReportsWriterFailure(t *testing.T) {
	boom := errors.New("disk full")
	w := &failingWriter{limit: 2, err: boom}
	err := Write(w, sampleFlows())
	if !errors.Is(err, boom) {
		t.Fatalf("error %v", err)
	}
	if w.writes < 2 {
		t.Fatalf("%d writes: the document was not streamed", w.writes)
	}
}

func TestWriteNoEntries(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, nil); err != nil {
		t.Fatal(err)
	}
	var doc testDocument
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("decode %q: %v", buf.String(), err)
	}
	if doc.Log.Version != "1.2" || len(doc.Log.Entries) != 0 {
		t.Fatalf("document %+v", doc.Log)
	}
}

// equalJSON compares two values by their JSON form.
func equalJSON(t *testing.T, a, b any) bool {
	t.Helper()
	left, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	right, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.Equal(left, right)
}

var _ io.Writer = (*failingWriter)(nil)
