package rules

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

func TestRewriteChain(t *testing.T) {
	e := New()
	mustSet(t, e,
		rewriteRule("to-backend", tool.RewriteEffect{URL: "http://backend.local/v2/items?page=2"}),
		rewriteRule("auth", tool.RewriteEffect{RequestHeaders: []tool.HeaderEdit{
			{Name: "x-token", Value: "abc"},
			{Name: "X-Drop", Remove: true},
		}}),
		rewriteRule("scrub", tool.RewriteEffect{RequestBody: &tool.BodyEdit{Find: "s3cret", Replace: "redacted"}}),
	)

	req := newRequest(t, http.MethodPost, "https://api.example.com/v1/items?page=1",
		strings.NewReader(`{"secret":"s3cret","keep":true}`))
	req.Header.Set("X-Drop", "yes")
	v, err := e.Request(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if got := req.URL.String(); got != "http://backend.local/v2/items?page=2" {
		t.Errorf("URL is %q, want the rewritten one", got)
	}
	if req.Host != "backend.local" {
		t.Errorf("Host is %q, want backend.local", req.Host)
	}
	if got := req.Header.Get("X-Token"); got != "abc" {
		t.Errorf("X-Token is %q, want abc", got)
	}
	if _, held := req.Header["X-Drop"]; held {
		t.Error("X-Drop was not removed")
	}
	want := `{"secret":"redacted","keep":true}`
	if got := readAll(t, req.Body); got != want {
		t.Errorf("body is %q, want %q", got, want)
	}
	if req.ContentLength != int64(len(want)) {
		t.Errorf("ContentLength is %d, want %d", req.ContentLength, len(want))
	}
	if got := req.Header.Get("Content-Length"); got != strconv.Itoa(len(want)) {
		t.Errorf("Content-Length header is %q, want %d", got, len(want))
	}
	if !slices.Equal(v.Hits, []string{"to-backend", "auth", "scrub"}) {
		t.Errorf("hits %v, want the three rewrites in order", v.Hits)
	}
}

func TestBodyEdits(t *testing.T) {
	tests := []struct {
		name string
		edit tool.BodyEdit
		body string
		want string
	}{
		{"whole body", tool.BodyEdit{Replace: "replaced"}, "original", "replaced"},
		{"literal", tool.BodyEdit{Find: "ab", Replace: "X"}, "aab ab", "aX X"},
		{"regexp group", tool.BodyEdit{Find: `(\w+)=(\d+)`, Replace: "$2=$1", Regexp: true}, "a=1 b=2", "1=a 2=b"},
		{"regexp without a match", tool.BodyEdit{Find: `\d+`, Replace: "n", Regexp: true}, "abc", "abc"},
		{"empty body", tool.BodyEdit{Find: "a", Replace: "b"}, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := New()
			mustSet(t, e, rewriteRule("edit", tool.RewriteEffect{RequestBody: &tt.edit}))
			req := newRequest(t, http.MethodPost, "https://example.com/x", strings.NewReader(tt.body))
			if _, err := e.Request(req); err != nil {
				t.Fatalf("request: %v", err)
			}
			if got := readAll(t, req.Body); got != tt.want {
				t.Fatalf("body is %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDelaysAccumulateUntilTheFirstMock(t *testing.T) {
	e := New()
	mustSet(t, e,
		delayRule("d1", 100*time.Millisecond),
		delayRule("d2", 250*time.Millisecond),
		mockRule("m1", tool.MockEffect{Body: "first"}),
		mockRule("m2", tool.MockEffect{Body: "second"}),
		delayRule("d3", time.Second),
	)
	v, err := e.Request(newRequest(t, http.MethodGet, "https://example.com/x", nil))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if v.Delay != 350*time.Millisecond {
		t.Errorf("delay is %v, want 350ms", v.Delay)
	}
	if v.Mock == nil {
		t.Fatal("no mock was made")
	}
	if got := readAll(t, v.Mock.Body); got != "first" {
		t.Errorf("mock body is %q, want the first mock's", got)
	}
	if !slices.Equal(v.Hits, []string{"d1", "d2", "m1"}) {
		t.Errorf("hits %v, want the rules up to the first mock", v.Hits)
	}
}

func TestRewriteBeforeAMockStillCounts(t *testing.T) {
	e := New()
	mustSet(t, e,
		rewriteRule("rw", tool.RewriteEffect{RequestHeaders: []tool.HeaderEdit{{Name: "X-Rw", Value: "1"}}}),
		mockRule("m", tool.MockEffect{Body: "mocked"}),
	)
	req := newRequest(t, http.MethodGet, "https://example.com/x", nil)
	v, err := e.Request(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if req.Header.Get("X-Rw") != "1" {
		t.Error("the rewrite before the mock did not apply")
	}
	if !slices.Equal(v.Hits, []string{"rw", "m"}) {
		t.Errorf("hits %v, want the rewrite then the mock", v.Hits)
	}
}

func TestBlockEndsTheBook(t *testing.T) {
	e := New()
	mustSet(t, e, blockRule("b"), mockRule("m", tool.MockEffect{Body: "never"}))
	v, err := e.Request(newRequest(t, http.MethodGet, "https://example.com/x", nil))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if !v.Block {
		t.Error("the request was not blocked")
	}
	if v.Mock != nil {
		t.Error("a rule after the block applied")
	}
	if !slices.Equal(v.Hits, []string{"b"}) {
		t.Errorf("hits %v, want only the block", v.Hits)
	}
}

func TestMockResponse(t *testing.T) {
	e := New()
	mustSet(t, e, mockRule("m", tool.MockEffect{
		Status:  http.StatusCreated,
		Headers: []tool.Header{{Name: "X-A", Value: "b"}, {Name: "Content-Type", Value: "application/json"}},
		Body:    `{"ok":true}`,
	}))
	req := newRequest(t, http.MethodGet, "https://example.com/x", nil)
	v, err := e.Request(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp := v.Mock
	if resp == nil {
		t.Fatal("no mock was made")
	}
	if resp.StatusCode != http.StatusCreated || resp.Status != "201 Created" {
		t.Errorf("status is %d %q", resp.StatusCode, resp.Status)
	}
	if resp.Header.Get("X-A") != "b" || resp.Header.Get("Content-Type") != "application/json" {
		t.Errorf("headers are %v", resp.Header)
	}
	if resp.ContentLength != 11 || resp.Header.Get("Content-Length") != "11" {
		t.Errorf("length is %d, header %q", resp.ContentLength, resp.Header.Get("Content-Length"))
	}
	if got := readAll(t, resp.Body); got != `{"ok":true}` {
		t.Errorf("body is %q", got)
	}
	if resp.Request != req {
		t.Error("the mock does not point at its request")
	}
}

func TestMockDefaults(t *testing.T) {
	e := New()
	mustSet(t, e, mockRule("m", tool.MockEffect{Body: "plain"}))
	v, err := e.Request(newRequest(t, http.MethodGet, "https://example.com/x", nil))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if v.Mock.StatusCode != http.StatusOK || v.Mock.Status != "200 OK" {
		t.Errorf("status is %d %q, want 200 OK", v.Mock.StatusCode, v.Mock.Status)
	}
	if got := v.Mock.Header.Get("Content-Type"); got != defaultContentType {
		t.Errorf("Content-Type is %q, want %q", got, defaultContentType)
	}
}

func TestMockServesAFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "answer.json")
	if err := os.WriteFile(path, []byte(`{"from":"file"}`), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	e := New()
	mustSet(t, e, mockRule("m", tool.MockEffect{File: path}))
	v, err := e.Request(newRequest(t, http.MethodGet, "https://example.com/x", nil))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if got := readAll(t, v.Mock.Body); got != `{"from":"file"}` {
		t.Fatalf("body is %q, want the file's", got)
	}
}

func TestMockFileMissingIsAnError(t *testing.T) {
	e := New()
	mustSet(t, e, mockRule("m", tool.MockEffect{File: filepath.Join(t.TempDir(), "gone")}))
	v, err := e.Request(newRequest(t, http.MethodGet, "https://example.com/x", nil))
	if err == nil {
		t.Fatal("a missing mock file went unnoticed")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("error %v does not wrap the read failure", err)
	}
	if !strings.HasPrefix(err.Error(), "m: ") {
		t.Fatalf("error %q does not name the rule", err)
	}
	if v.Mock != nil || len(v.Hits) != 0 {
		t.Fatalf("the failed mock applied: %+v", v)
	}
}

func TestRuleWithoutANameIsHitByID(t *testing.T) {
	e := New()
	unnamed := blockRule("id-only")
	unnamed.Name = ""
	mustSet(t, e, unnamed)
	v, err := e.Request(newRequest(t, http.MethodGet, "https://example.com/x", nil))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if !slices.Equal(v.Hits, []string{"id-only"}) {
		t.Fatalf("hits %v, want the rule's ID", v.Hits)
	}
}

func TestBodyOverTheLimitSkipsTheWholeRule(t *testing.T) {
	e := New()
	mustSet(t, e, rewriteRule("big", tool.RewriteEffect{
		RequestHeaders: []tool.HeaderEdit{{Name: "X-Skipped", Value: "1"}},
		RequestBody:    &tool.BodyEdit{Replace: "small"},
	}))
	body := bytes.Repeat([]byte("a"), tool.MaxBody+1)
	req := newRequest(t, http.MethodPost, "https://example.com/upload", bytes.NewReader(body))
	v, err := e.Request(req)
	if !errors.Is(err, ErrBodyTooLarge) {
		t.Fatalf("error is %v, want ErrBodyTooLarge", err)
	}
	if got := req.Header.Get("X-Skipped"); got != "" {
		t.Error("the skipped rule still edited the request")
	}
	if len(v.Hits) != 0 {
		t.Fatalf("hits %v, want none", v.Hits)
	}
	got, readErr := io.ReadAll(req.Body)
	if readErr != nil {
		t.Fatalf("read restored body: %v", readErr)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("the request body lost bytes: %d of %d", len(got), len(body))
	}
}

func TestRequestHeaderEditsIgnoreCase(t *testing.T) {
	e := New()
	mustSet(t, e, rewriteRule("h", tool.RewriteEffect{RequestHeaders: []tool.HeaderEdit{
		{Name: "X-DROP", Remove: true},
		{Name: "x-set", Value: "v"},
	}}))
	req := newRequest(t, http.MethodGet, "https://example.com/x", nil)
	lower := "x-drop" // a message built by hand may hold a header in any case
	req.Header[lower] = []string{"1"}
	if _, err := e.Request(req); err != nil {
		t.Fatalf("request: %v", err)
	}
	if _, held := req.Header[lower]; held {
		t.Error("a header set in another case was not removed")
	}
	if got := req.Header.Get("X-Set"); got != "v" {
		t.Errorf("X-Set is %q, want v", got)
	}
}

func TestRequestHostHeaderEditReachesReqHost(t *testing.T) {
	e := New()
	mustSet(t, e, rewriteRule("h", tool.RewriteEffect{RequestHeaders: []tool.HeaderEdit{
		{Name: "Host", Value: "other.example"},
	}}))
	req := newRequest(t, http.MethodGet, "https://example.com/x", nil)
	if _, err := e.Request(req); err != nil {
		t.Fatalf("request: %v", err)
	}
	if req.Host != "other.example" {
		t.Fatalf("req.Host is %q, want the edited one", req.Host)
	}
}

func TestRequestBodyReadErrorIsReported(t *testing.T) {
	boom := errors.New("cut stream")
	e := New()
	mustSet(t, e, rewriteRule("rw", tool.RewriteEffect{
		RequestHeaders: []tool.HeaderEdit{{Name: "X-Skipped", Value: "1"}},
		RequestBody:    &tool.BodyEdit{Replace: "x"},
	}))
	req := newRequest(t, http.MethodPost, "https://example.com/x", nil)
	req.Body = &brokenBody{data: []byte("abc"), err: boom}
	req.ContentLength = 3
	v, err := e.Request(req)
	if !errors.Is(err, boom) {
		t.Fatalf("error is %v, want the read failure", err)
	}
	if req.Header.Get("X-Skipped") != "" {
		t.Error("the skipped rule still edited the request")
	}
	if len(v.Hits) != 0 {
		t.Fatalf("hits %v, want none", v.Hits)
	}
	got := make([]byte, 3)
	if _, err := io.ReadFull(req.Body, got); err != nil {
		t.Fatalf("read restored body: %v", err)
	}
	if string(got) != "abc" {
		t.Fatalf("restored bytes are %q, want abc", got)
	}
}

func TestRestoredBodyStillClosesTheOriginal(t *testing.T) {
	e := New()
	mustSet(t, e, rewriteRule("big", tool.RewriteEffect{RequestBody: &tool.BodyEdit{Replace: "x"}}))
	original := &watchedBody{Reader: bytes.NewReader(bytes.Repeat([]byte("a"), tool.MaxBody+1))}
	req := newRequest(t, http.MethodPost, "https://example.com/x", nil)
	req.Body = original
	req.ContentLength = tool.MaxBody + 1
	if _, err := e.Request(req); !errors.Is(err, ErrBodyTooLarge) {
		t.Fatalf("error is %v, want ErrBodyTooLarge", err)
	}
	if err := req.Body.Close(); err != nil {
		t.Fatalf("close restored body: %v", err)
	}
	if !original.closed {
		t.Fatal("closing the restored body did not close the original")
	}
}

func TestRequestHostHeaderRemoval(t *testing.T) {
	e := New()
	mustSet(t, e, rewriteRule("h", tool.RewriteEffect{RequestHeaders: []tool.HeaderEdit{
		{Name: "host", Remove: true},
	}}))
	req := newRequest(t, http.MethodGet, "https://example.com/x", nil)
	req.Host = "example.com"
	if _, err := e.Request(req); err != nil {
		t.Fatalf("request: %v", err)
	}
	if req.Host != "" {
		t.Fatalf("req.Host is %q, want it removed", req.Host)
	}
}

func TestMockRefusesADirectory(t *testing.T) {
	e := New()
	mustSet(t, e, mockRule("m", tool.MockEffect{File: t.TempDir()}))
	v, err := e.Request(newRequest(t, http.MethodGet, "https://example.com/x", nil))
	if err == nil {
		t.Fatal("a directory was served as a mock body")
	}
	if !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("error %q does not say why the file was refused", err)
	}
	if v.Mock != nil || len(v.Hits) != 0 {
		t.Fatalf("the refused mock applied: %+v", v)
	}
}

func TestMockRefusesADevice(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no /dev/null on Windows")
	}
	path := filepath.Join(t.TempDir(), "null")
	if err := os.Symlink("/dev/null", path); err != nil {
		t.Skipf("cannot make a link to a device: %v", err)
	}
	e := New()
	mustSet(t, e, mockRule("m", tool.MockEffect{File: path}))
	// Before the stat check this read the device, which never ends for
	// /dev/zero and hides a body that cannot be taken whole.
	v, err := e.Request(newRequest(t, http.MethodGet, "https://example.com/x", nil))
	if err == nil {
		t.Fatal("a device was served as a mock body")
	}
	if !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("error %q does not say why the file was refused", err)
	}
	if v.Mock != nil || len(v.Hits) != 0 {
		t.Fatalf("the refused mock applied: %+v", v)
	}
}

func TestMockRefusesAFileOverTheLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.bin")
	big := bytes.Repeat([]byte("a"), tool.MaxBody+1)
	if err := os.WriteFile(path, big, 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	e := New()
	mustSet(t, e, mockRule("m", tool.MockEffect{File: path}))
	v, err := e.Request(newRequest(t, http.MethodGet, "https://example.com/x", nil))
	if !errors.Is(err, ErrBodyTooLarge) {
		t.Fatalf("error is %v, want ErrBodyTooLarge", err)
	}
	if v.Mock != nil || len(v.Hits) != 0 {
		t.Fatalf("an oversized mock was served: %+v", v)
	}
}
