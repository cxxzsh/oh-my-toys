package rules

import (
	"context"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// newRequest builds the request a test sends.
func newRequest(t *testing.T, method, target string, body io.Reader) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), method, target, body)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	return req
}

// readAll reads a body whole.
func readAll(t *testing.T, body io.Reader) string {
	t.Helper()
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(data)
}

// mustSet puts rules into an engine, failing the test when they are bad.
func mustSet(t *testing.T, e *Engine, rules ...tool.Rule) {
	t.Helper()
	if err := e.Set(rules); err != nil {
		t.Fatalf("set rules: %v", err)
	}
}

// markRule is a rewrite that sets X-Hit on the requests it matches.
func markRule(m tool.RuleMatch) tool.Rule {
	return tool.Rule{
		ID: "mark", Name: "mark", Enabled: true, Kind: tool.RuleRewrite,
		Match:   m,
		Rewrite: tool.RewriteEffect{RequestHeaders: []tool.HeaderEdit{{Name: "X-Hit", Value: "1"}}},
	}
}

// rewriteRule is a rewrite that matches every request.
func rewriteRule(id string, eff tool.RewriteEffect) tool.Rule {
	return tool.Rule{
		ID: id, Name: id, Enabled: true, Kind: tool.RuleRewrite,
		Match: tool.RuleMatch{Pattern: "*"}, Rewrite: eff,
	}
}

// mockRule is a mock that matches every request.
func mockRule(id string, eff tool.MockEffect) tool.Rule {
	return tool.Rule{
		ID: id, Name: id, Enabled: true, Kind: tool.RuleMock,
		Match: tool.RuleMatch{Pattern: "*"}, Mock: eff,
	}
}

// delayRule holds every response back by hold.
func delayRule(id string, hold time.Duration) tool.Rule {
	return tool.Rule{
		ID: id, Name: id, Enabled: true, Kind: tool.RuleDelay,
		Match: tool.RuleMatch{Pattern: "*"}, Delay: hold,
	}
}

// blockRule closes the connection for every request.
func blockRule(id string) tool.Rule {
	return tool.Rule{
		ID: id, Name: id, Enabled: true, Kind: tool.RuleBlock,
		Match: tool.RuleMatch{Pattern: "*"},
	}
}

func TestSetRejectsBadRules(t *testing.T) {
	ok := tool.Rule{ID: "ok", Name: "ok", Enabled: true, Match: tool.RuleMatch{Pattern: "*"}}
	tests := []struct {
		name  string
		rules []tool.Rule
		want  string
	}{
		{"empty ID", []tool.Rule{{Name: "no id", Match: tool.RuleMatch{Pattern: "*"}}}, "needs an ID"},
		{"duplicate ID", []tool.Rule{ok, ok}, "repeats the ID"},
		{"empty pattern", []tool.Rule{{ID: "a"}}, "needs a pattern"},
		{"bad pattern", []tool.Rule{{ID: "a", Match: tool.RuleMatch{Pattern: "a(", Regexp: true}}}, "does not compile"},
		{"bad body pattern", []tool.Rule{{
			ID: "a", Match: tool.RuleMatch{Pattern: "*"},
			Rewrite: tool.RewriteEffect{RequestBody: &tool.BodyEdit{Find: "(", Regexp: true}},
		}}, "body pattern does not compile"},
		{"bad response body pattern", []tool.Rule{{
			ID: "a", Match: tool.RuleMatch{Pattern: "*"},
			Rewrite: tool.RewriteEffect{ResponseBody: &tool.BodyEdit{Find: "[", Regexp: true}},
		}}, "body pattern does not compile"},
		{"status below", []tool.Rule{{
			ID: "a", Match: tool.RuleMatch{Pattern: "*"}, Rewrite: tool.RewriteEffect{Status: 99},
		}}, "status 99 is not between 200 and 599"},
		{"interim rewrite status", []tool.Rule{{
			ID: "a", Match: tool.RuleMatch{Pattern: "*"}, Rewrite: tool.RewriteEffect{Status: 101},
		}}, "status 101 is not between 200 and 599"},
		{"interim mock status", []tool.Rule{{
			ID: "a", Match: tool.RuleMatch{Pattern: "*"}, Mock: tool.MockEffect{Status: 103},
		}}, "status 103 is not between 200 and 599"},
		{"status above", []tool.Rule{{
			ID: "a", Match: tool.RuleMatch{Pattern: "*"}, Mock: tool.MockEffect{Status: 600},
		}}, "status 600 is not between 200 and 599"},
		{"negative delay", []tool.Rule{{
			ID: "a", Match: tool.RuleMatch{Pattern: "*"}, Delay: -time.Second,
		}}, "delay -1s is not between 0 and 5m0s"},
		{"long delay", []tool.Rule{{
			ID: "a", Match: tool.RuleMatch{Pattern: "*"}, Delay: 6 * time.Minute,
		}}, "delay 6m0s is not between 0 and 5m0s"},
		{"bad URL", []tool.Rule{{
			ID: "a", Match: tool.RuleMatch{Pattern: "*"}, Rewrite: tool.RewriteEffect{URL: "://bad"},
		}}, "does not parse"},
		{"empty request header name", []tool.Rule{{
			ID: "a", Match: tool.RuleMatch{Pattern: "*"},
			Rewrite: tool.RewriteEffect{RequestHeaders: []tool.HeaderEdit{{Name: "", Value: "x"}}},
		}}, `header name "" is not a valid field name`},
		{"request header name with a space", []tool.Rule{{
			ID: "a", Match: tool.RuleMatch{Pattern: "*"},
			Rewrite: tool.RewriteEffect{RequestHeaders: []tool.HeaderEdit{{Name: "bad name", Value: "x"}}},
		}}, `header name "bad name"`},
		{"request header value with a line break", []tool.Rule{{
			ID: "a", Match: tool.RuleMatch{Pattern: "*"},
			Rewrite: tool.RewriteEffect{RequestHeaders: []tool.HeaderEdit{{Name: "X-A", Value: "a\r\nb"}}},
		}}, "may not be sent"},
		{"response header value with a line break", []tool.Rule{{
			ID: "a", Match: tool.RuleMatch{Pattern: "*"},
			Rewrite: tool.RewriteEffect{ResponseHeaders: []tool.HeaderEdit{{Name: "X-A", Value: "a\nb"}}},
		}}, "may not be sent"},
		{"mock header value with a line break", []tool.Rule{{
			ID: "a", Match: tool.RuleMatch{Pattern: "*"},
			Mock: tool.MockEffect{Headers: []tool.Header{{Name: "X-A", Value: "a\r\nb"}}},
		}}, "may not be sent"},
		{"mock header name with a space", []tool.Rule{{
			ID: "a", Match: tool.RuleMatch{Pattern: "*"},
			Mock: tool.MockEffect{Headers: []tool.Header{{Name: "bad name", Value: "x"}}},
		}}, `header name "bad name"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := New()
			err := e.Set(tt.rules)
			if err == nil {
				t.Fatal("Set accepted an invalid rule")
			}
			if !errors.Is(err, ErrInvalidRule) {
				t.Fatalf("error %v is not ErrInvalidRule", err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error %q does not mention %q", err, tt.want)
			}
			if got := e.Get(); got != nil {
				t.Fatalf("a failed Set left %d rules behind", len(got))
			}
		})
	}
}

func TestSetErrorNamesTheRule(t *testing.T) {
	e := New()
	err := e.Set([]tool.Rule{{
		ID: "a", Name: "block ads", Match: tool.RuleMatch{Pattern: "*"}, Delay: time.Hour,
	}})
	if err == nil || !strings.Contains(err.Error(), `rule "block ads"`) {
		t.Fatalf("error %v does not name the rule", err)
	}
}

func TestSetKeepsTheOldBookOnFailure(t *testing.T) {
	e := New()
	mustSet(t, e, markRule(tool.RuleMatch{Pattern: "*"}))
	bad := tool.Rule{ID: "bad", Name: "bad", Match: tool.RuleMatch{Pattern: "*"}, Delay: time.Hour}
	if err := e.Set([]tool.Rule{bad}); err == nil {
		t.Fatal("Set accepted a bad rule")
	}
	req := newRequest(t, http.MethodGet, "https://example.com/x", nil)
	if _, err := e.Request(req); err != nil {
		t.Fatalf("request after a failed Set: %v", err)
	}
	if got := req.Header.Get("X-Hit"); got != "1" {
		t.Fatalf("the old book was lost: X-Hit is %q", got)
	}
	if got := len(e.Get()); got != 1 {
		t.Fatalf("the book holds %d rules, want 1", got)
	}
}

func TestGetReturnsACopy(t *testing.T) {
	e := New()
	methods := []string{http.MethodGet}
	headers := []tool.HeaderEdit{{Name: "X-From", Value: "book"}}
	rules := []tool.Rule{{
		ID: "a", Name: "a", Enabled: true, Kind: tool.RuleRewrite,
		Match:   tool.RuleMatch{Methods: methods, Pattern: "example.com/*"},
		Rewrite: tool.RewriteEffect{RequestHeaders: headers},
	}}
	mustSet(t, e, rules...)

	// Neither the caller's slices nor the returned ones may reach the book.
	methods[0], headers[0].Value, rules[0].Match.Pattern = http.MethodPost, "changed", "other.com/*"
	got := e.Get()
	if len(got) != 1 {
		t.Fatalf("Get returned %d rules, want 1", len(got))
	}
	if got[0].Match.Methods[0] != http.MethodGet || got[0].Rewrite.RequestHeaders[0].Value != "book" ||
		got[0].Match.Pattern != "example.com/*" {
		t.Fatalf("the caller's change reached the book: %+v", got[0])
	}
	got[0].Match.Methods[0], got[0].Rewrite.RequestHeaders[0].Value = http.MethodPut, "changed"

	req := newRequest(t, http.MethodGet, "https://example.com/x", nil)
	if _, err := e.Request(req); err != nil {
		t.Fatalf("request: %v", err)
	}
	if got := req.Header.Get("X-From"); got != "book" {
		t.Fatalf("X-From is %q, want book: Get handed out the book itself", got)
	}
}

func TestDisabledRuleIsIgnored(t *testing.T) {
	e := New()
	off := mockRule("off", tool.MockEffect{Body: "nope"})
	off.Enabled = false
	mustSet(t, e, off)

	v, err := e.Request(newRequest(t, http.MethodGet, "https://example.com/x", nil))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if v.Mock != nil || v.Block || v.Delay != 0 || len(v.Hits) != 0 {
		t.Fatalf("a disabled rule applied: %+v", v)
	}
}

func TestUnknownKindDoesNothing(t *testing.T) {
	e := New()
	other := tool.Rule{ID: "odd", Name: "odd", Enabled: true, Kind: tool.RuleKind(99), Match: tool.RuleMatch{Pattern: "*"}}
	mustSet(t, e, other)
	v, err := e.Request(newRequest(t, http.MethodGet, "https://example.com/x", nil))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if v.Mock != nil || v.Block || len(v.Hits) != 0 {
		t.Fatalf("a rule of an unknown kind applied: %+v", v)
	}
}

func TestConcurrentRequestAndSet(t *testing.T) {
	e := New()
	books := [][]tool.Rule{
		{rewriteRule("a", tool.RewriteEffect{
			RequestHeaders: []tool.HeaderEdit{{Name: "X-Book", Value: "one"}},
		})},
		{
			rewriteRule("a", tool.RewriteEffect{RequestHeaders: []tool.HeaderEdit{{Name: "X-Book", Value: "two"}}}),
			delayRule("b", time.Millisecond),
		},
	}
	mustSet(t, e, books[0]...)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hammer(t, e, stop)
		}()
	}
	for i := range 200 {
		if err := e.Set(books[i%2]); err != nil {
			t.Fatalf("set rules: %v", err)
		}
	}
	close(stop)
	wg.Wait()
}

// hammer requests and reads the book until stop is closed.
func hammer(t *testing.T, e *Engine, stop <-chan struct{}) {
	for {
		select {
		case <-stop:
			return
		default:
		}
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.com/x", nil)
		if err != nil {
			t.Errorf("build request: %v", err)
			return
		}
		if _, err := e.Request(req); err != nil {
			t.Errorf("request: %v", err)
			return
		}
		_ = e.Get()
	}
}

// brokenBody hands out its bytes and then fails, as a cut stream does.
type brokenBody struct {
	data []byte
	err  error
}

// Read hands out what is left, then the failure.
func (b *brokenBody) Read(p []byte) (int, error) {
	if len(b.data) == 0 {
		return 0, b.err
	}
	n := copy(p, b.data)
	b.data = b.data[n:]
	return n, nil
}

// Close does nothing; the test watches the read, not the close.
func (b *brokenBody) Close() error { return nil }

// watchedBody is a body that records being closed.
type watchedBody struct {
	io.Reader
	closed bool
}

// Close records that the body was closed.
func (w *watchedBody) Close() error {
	w.closed = true
	return nil
}

func TestHitsHaveNoDuplicates(t *testing.T) {
	e := New()
	first := rewriteRule("a", tool.RewriteEffect{RequestHeaders: []tool.HeaderEdit{{Name: "X-A", Value: "1"}}})
	second := rewriteRule("b", tool.RewriteEffect{RequestHeaders: []tool.HeaderEdit{{Name: "X-B", Value: "1"}}})
	first.Name, second.Name = "same", "same"
	mustSet(t, e, first, second)
	req := newRequest(t, http.MethodGet, "https://example.com/x", nil)
	v, err := e.Request(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if req.Header.Get("X-A") != "1" || req.Header.Get("X-B") != "1" {
		t.Error("both rules should have applied")
	}
	if !slices.Equal(v.Hits, []string{"same"}) {
		t.Fatalf("hits %v, want the shared name once", v.Hits)
	}
}

func TestMockWithoutAStandardStatusText(t *testing.T) {
	e := New()
	mustSet(t, e, mockRule("m", tool.MockEffect{Status: 250, Body: "x"}))
	v, err := e.Request(newRequest(t, http.MethodGet, "https://example.com/x", nil))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if v.Mock.Status != "250" {
		t.Fatalf("status is %q, want the bare code", v.Mock.Status)
	}
}

func TestNilAndURLlessRequests(t *testing.T) {
	e := New()
	mustSet(t, e, markRule(tool.RuleMatch{Pattern: "*"}))
	if v, err := e.Request(nil); err != nil || v.Mock != nil || v.Block || len(v.Hits) != 0 {
		t.Fatalf("a nil request gave %+v, %v", v, err)
	}
	if hits, err := e.Response(nil, nil); err != nil || len(hits) != 0 {
		t.Fatalf("nil arguments gave %v, %v", hits, err)
	}
	req := &http.Request{Method: http.MethodGet}
	if v, err := e.Request(req); err != nil || len(v.Hits) != 0 {
		t.Fatalf("a request without a URL matched something: %+v, %v", v, err)
	}
}
