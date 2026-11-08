package rules

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

func TestPatternMatching(t *testing.T) {
	tests := []struct {
		name   string
		match  tool.RuleMatch
		method string
		target string
		want   bool
	}{
		{
			name: "exact host and path", match: tool.RuleMatch{Pattern: "example.com/a"},
			method: http.MethodGet, target: "https://example.com/a", want: true,
		},
		{
			name: "anchored at the end", match: tool.RuleMatch{Pattern: "example.com/a"},
			method: http.MethodGet, target: "https://example.com/a/b", want: false,
		},
		{
			name: "anchored at the start", match: tool.RuleMatch{Pattern: "/a"},
			method: http.MethodGet, target: "https://example.com/a", want: false,
		},
		{
			name: "glob crosses slashes", match: tool.RuleMatch{Pattern: "example.com/*/c"},
			method: http.MethodGet, target: "https://example.com/a/b/c", want: true,
		},
		{
			name: "glob part missing", match: tool.RuleMatch{Pattern: "example.com/*/c"},
			method: http.MethodGet, target: "https://example.com/a/b", want: false,
		},
		{
			name: "glob middle missing", match: tool.RuleMatch{Pattern: "example.com/*/needle/*"},
			method: http.MethodGet, target: "https://example.com/a/b", want: false,
		},
		{
			name: "consecutive stars", match: tool.RuleMatch{Pattern: "example.com/**/a"},
			method: http.MethodGet, target: "https://example.com/x/y/a", want: true,
		},
		{
			name: "glob at both ends", match: tool.RuleMatch{Pattern: "*example.com/a*"},
			method: http.MethodGet, target: "https://api.example.com/abc", want: true,
		},
		{
			name: "star alone matches anything", match: tool.RuleMatch{Pattern: "*"},
			method: http.MethodOptions, target: "https://example.com/anything?x=1", want: true,
		},
		{
			name: "full pattern with the scheme", match: tool.RuleMatch{Pattern: "https://example.com/a"},
			method: http.MethodGet, target: "https://example.com/a", want: true,
		},
		{
			name: "scheme-less pattern ignores the scheme", match: tool.RuleMatch{Pattern: "example.com/a"},
			method: http.MethodGet, target: "http://example.com/a", want: true,
		},
		{
			name: "scheme in the pattern still matters", match: tool.RuleMatch{Pattern: "https://example.com/a"},
			method: http.MethodGet, target: "http://example.com/a", want: false,
		},
		{
			name: "host and port", match: tool.RuleMatch{Pattern: "example.com:8080/a"},
			method: http.MethodGet, target: "http://example.com:8080/a", want: true,
		},
		{
			name: "another host", match: tool.RuleMatch{Pattern: "other.com/*"},
			method: http.MethodGet, target: "https://example.com/a", want: false,
		},
		{
			name: "the query is part of the target", match: tool.RuleMatch{Pattern: "example.com/a?x=1"},
			method: http.MethodGet, target: "https://example.com/a?x=1", want: true,
		},
		{
			name: "another query value", match: tool.RuleMatch{Pattern: "example.com/a?x=1"},
			method: http.MethodGet, target: "https://example.com/a?x=2", want: false,
		},
		{
			name: "glob in the query", match: tool.RuleMatch{Pattern: "example.com/a?x=*"},
			method: http.MethodGet, target: "https://example.com/a?x=9", want: true,
		},
		{
			name: "regexp is not anchored", match: tool.RuleMatch{Pattern: `example\.com/a`, Regexp: true},
			method: http.MethodGet, target: "https://example.com/abc", want: true,
		},
		{
			name: "regexp may anchor itself", match: tool.RuleMatch{Pattern: `^example\.com/a$`, Regexp: true},
			method: http.MethodGet, target: "https://example.com/a", want: true,
		},
		{
			name: "anchored regexp rejects a longer path", match: tool.RuleMatch{Pattern: `^example\.com/a$`, Regexp: true},
			method: http.MethodGet, target: "https://example.com/ab", want: false,
		},
		{
			name: "regexp with the scheme", match: tool.RuleMatch{Pattern: `^https://example\.com/`, Regexp: true},
			method: http.MethodGet, target: "https://example.com/x", want: true,
		},
		{
			name: "method membership ignores case", match: tool.RuleMatch{Pattern: "*", Methods: []string{"get"}},
			method: http.MethodGet, target: "https://example.com/a", want: true,
		},
		{
			name: "method not listed", match: tool.RuleMatch{Pattern: "*", Methods: []string{"GET", "PUT"}},
			method: http.MethodPost, target: "https://example.com/a", want: false,
		},
		{
			name: "no method matches any", match: tool.RuleMatch{Pattern: "*"},
			method: http.MethodPatch, target: "https://example.com/a", want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := New()
			mustSet(t, e, markRule(tt.match))
			req := newRequest(t, tt.method, tt.target, nil)
			v, err := e.Request(req)
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			got := req.Header.Get("X-Hit") == "1"
			if got != tt.want {
				t.Fatalf("pattern %q matched %v, want %v (hits %v)", tt.match.Pattern, got, tt.want, v.Hits)
			}
		})
	}
}

func TestPatternUsesRequestHostWhenTheURLHasNone(t *testing.T) {
	e := New()
	mustSet(t, e, markRule(tool.RuleMatch{Pattern: "example.com/a"}))
	req := &http.Request{
		Method: http.MethodGet,
		URL:    &url.URL{Path: "/a"},
		Host:   "example.com",
	}
	if _, err := e.Request(req); err != nil {
		t.Fatalf("request: %v", err)
	}
	if got := req.Header.Get("X-Hit"); got != "1" {
		t.Fatal("the pattern did not use req.Host")
	}
}
