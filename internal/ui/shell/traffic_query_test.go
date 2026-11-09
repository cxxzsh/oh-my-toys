package shell

import (
	"reflect"
	"testing"
)

// parseCase is one search text and what parseQuery must make of it.
type parseCase struct {
	name     string
	text     string
	words    []string
	hosts    []string
	methods  []string
	programs []string
	failed   bool
	tokens   int
}

// TestParseQuery pins how the search text is read: which pieces are tokens,
// which are plain words, and what each token records.
func TestParseQuery(t *testing.T) {
	tests := []parseCase{
		{name: "empty", text: ""},
		{name: "only white space", text: " \t\n "},
		{name: "plain words are lower-cased", text: "Hello WORLD", words: []string{"hello", "world"}},
		{name: "a bare host is a word", text: "api.example.com", words: []string{"api.example.com"}},
		{name: "host token, key and value folded", text: "Host:API.Example.com", hosts: []string{"api.example.com"}, tokens: 1},
		{name: "method comma list", text: "method:POST,get", methods: []string{"POST", "GET"}, tokens: 1},
		{name: "upper-case key", text: "METHOD:post", methods: []string{"POST"}, tokens: 1},
		{name: "program value kept as typed", text: "program:Safari", programs: []string{"Safari"}, tokens: 1},
		{name: "quoted program keeps its space", text: `program:"Code Helper"`, programs: []string{"Code Helper"}, tokens: 1},
		{name: "unclosed quote runs to the end", text: `program:"Code`, programs: []string{"Code"}, tokens: 1},
		{name: "failed is case-insensitive", text: "failed:TRUE", failed: true, tokens: 1},
		{name: "failed with another value is a word", text: "failed:no", words: []string{"failed:no"}},
		{name: "unknown key is a word", text: "color:red", words: []string{"color:red"}},
		{name: "empty value is a word", text: "host:", words: []string{"host:"}},
		{name: "bad status value is a word", text: "status:banana", words: []string{"status:banana"}},
		{name: "unknown type name is a word", text: "type:xml", words: []string{"type:xml"}},
		{name: "known type name is a token", text: "type:json", tokens: 1},
		{
			name:     "mixed tokens and words in order",
			text:     `program:Safari status:4xx login  host:api.x.io failed:true`,
			words:    []string{"login"},
			hosts:    []string{"api.x.io"},
			programs: []string{"Safari"},
			failed:   true,
			tokens:   4,
		},
		{name: "garbage does not panic", text: `::: "" "" :: "`, words: []string{":::", "::"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) { checkParsed(t, tc.text, tc) })
	}
}

// checkParsed compares what parseQuery makes of text with the expected case.
func checkParsed(t *testing.T, text string, want parseCase) {
	t.Helper()
	q := parseQuery(text)
	if !reflect.DeepEqual(q.words, want.words) {
		t.Errorf("words = %q, want %q", q.words, want.words)
	}
	if !reflect.DeepEqual(q.hosts, want.hosts) {
		t.Errorf("hosts = %q, want %q", q.hosts, want.hosts)
	}
	if !reflect.DeepEqual(q.methods, want.methods) {
		t.Errorf("methods = %q, want %q", q.methods, want.methods)
	}
	if !reflect.DeepEqual(q.programs, want.programs) {
		t.Errorf("programs = %q, want %q", q.programs, want.programs)
	}
	if q.failed != want.failed {
		t.Errorf("failed = %v, want %v", q.failed, want.failed)
	}
	if len(q.tokens) != want.tokens {
		t.Errorf("tokens = %d, want %d", len(q.tokens), want.tokens)
	}
}

// TestStatusToken: status:4xx,5xx and status:404 name what a flow may carry.
func TestStatusToken(t *testing.T) {
	tests := []struct {
		text string
		yes  []int
		no   []int
	}{
		{text: "status:4xx,5xx", yes: []int{404, 503}, no: []int{200, 0}},
		{text: "status:404", yes: []int{404}, no: []int{405, 200}},
		{text: "status:none", yes: []int{0}, no: []int{200}},
		{text: "status:banana", yes: []int{200, 0}},
	}
	for _, tc := range tests {
		q := parseQuery(tc.text)
		for _, s := range tc.yes {
			if !q.statusAllowed(s) {
				t.Errorf("%q: status %d should be allowed", tc.text, s)
			}
		}
		for _, s := range tc.no {
			if q.statusAllowed(s) {
				t.Errorf("%q: status %d should not be allowed", tc.text, s)
			}
		}
	}
}

// TestTypeAndHostTokens: type names follow the media type chips, host
// patterns take a * for any run of characters.
func TestTypeAndHostTokens(t *testing.T) {
	q := parseQuery("type:json,html")
	for _, media := range []string{"application/json; charset=utf-8", "text/html"} {
		if !q.typeAllowed(media) {
			t.Errorf("type:json,html should allow %q", media)
		}
	}
	if q.typeAllowed("image/png") {
		t.Error("type:json,html should not allow image/png")
	}
	h := parseQuery("host:*.example.com")
	if !hostAllowed(h.hosts, "api.example.com") {
		t.Error("host pattern *.example.com should match api.example.com")
	}
	if hostAllowed(h.hosts, "example.org") {
		t.Error("host pattern *.example.com should not match example.org")
	}
}
