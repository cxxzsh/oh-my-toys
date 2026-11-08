package shell

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// sampleRules is the book the rule tests start from: one rule of each
// kind the inspector draws differently.
func sampleRules() []tool.Rule {
	return []tool.Rule{
		{
			ID: "rule-1", Name: "Add a header", Enabled: true, Kind: tool.RuleRewrite,
			Match:   tool.RuleMatch{Pattern: "https://api.example.com/*", Methods: []string{"GET"}},
			Rewrite: tool.RewriteEffect{RequestHeaders: []tool.HeaderEdit{{Name: "X-Debug", Value: "1"}}},
		},
		{
			ID: "rule-2", Name: "Mock the page", Enabled: true, Kind: tool.RuleMock,
			Match: tool.RuleMatch{Pattern: "https://api.example.com/v1/*"},
			Mock:  tool.MockEffect{Status: 200, Body: `{"mocked":true}`},
		},
		{
			ID: "rule-3", Name: "Slow it", Enabled: true, Kind: tool.RuleDelay,
			Match: tool.RuleMatch{Pattern: "*://cdn.example.com/*"}, Delay: 300 * time.Millisecond,
		},
	}
}

// openRules opens the inspector on the rule book.
func openRules(t *testing.T, f *testTraffic) *ui.Tester {
	t.Helper()
	tt := openTraffic(t, f)
	if err := tt.Click(i18n.TrafficViewRules()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	return tt
}

// waitRules waits until the tool has been given a book of n rules, and
// returns the last book it kept.
func waitRules(t *testing.T, f *testTraffic, n int) []tool.Rule {
	t.Helper()
	deadline := time.Now().Add(trafficWait)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		books := len(f.ruleBooks)
		var last []tool.Rule
		if books > 0 {
			last = cloneRules(f.ruleBooks[books-1])
		}
		f.mu.Unlock()
		if books > 0 && len(last) == n {
			return last
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("the tool was never given a book of %d rules", n)
	return nil
}

// ruleIDs is a book's ids, in order.
func ruleIDs(book []tool.Rule) []string {
	out := make([]string, len(book))
	for i, r := range book {
		out[i] = r.ID
	}
	return out
}

// TestRulesListOrder: the book is listed in order, and the buttons move a
// rule where the person put it, because the order decides the match.
func TestRulesListOrder(t *testing.T) {
	f := newTestTraffic()
	f.rules = sampleRules()
	tt := openRules(t, f)
	first, okA := tt.Find(ruleLabel(f.rules[0]))
	last, okB := tt.Find(ruleLabel(f.rules[2]))
	if !okA || !okB || first.Y >= last.Y {
		t.Fatalf("the rules are not listed in order: %v, %v", first, last)
	}
	if err := tt.Click(i18n.TrafficRuleMoveDown()); err != nil {
		t.Fatal(err)
	}
	book := waitRules(t, f, 3)
	if got := ruleIDs(book); got[0] != "rule-2" || got[1] != "rule-1" || got[2] != "rule-3" {
		t.Errorf("after moving the first rule down: %v", got)
	}
}

// TestRulesAddAndDelete: a new rule of a kind appears at the end, and a
// delete takes it away after the person says so.
func TestRulesAddAndDelete(t *testing.T) {
	f := newTestTraffic()
	f.rules = sampleRules()
	tt := openRules(t, f)
	if err := tt.Click(i18n.TrafficNewRule()); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem(i18n.TrafficKindMock()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	book := waitRules(t, f, 4)
	if book[3].Kind != tool.RuleMock || book[3].Match.Pattern != "*" {
		t.Errorf("the new rule is %+v", book[3])
	}
	if err := tt.Click(i18n.TrafficRuleDelete()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click(i18n.TrafficRuleDeleteConfirm()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	book = waitRules(t, f, 3)
	if got := ruleIDs(book); len(got) != 3 || got[0] != "rule-1" || got[2] != "rule-3" {
		t.Errorf("after deleting the new rule: %v", got)
	}
}

// TestRuleFormsPerKind: each kind shows its own fields, and no others.
func TestRuleFormsPerKind(t *testing.T) {
	f := newTestTraffic()
	f.rules = sampleRules()
	tt := openRules(t, f)
	if !tt.HasText(i18n.TrafficRuleRedirect()) || !tt.HasText(strings.ToUpper(i18n.TrafficRuleReqHeaders())) {
		t.Errorf("the rewrite form is missing its fields: %q", tt.Texts())
	}
	if err := tt.Click(ruleLabel(f.rules[1])); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText(i18n.TrafficRuleMockFile()) || !tt.HasText(i18n.TrafficRuleAddHeader()) {
		t.Errorf("the mock form is missing its fields: %q", tt.Texts())
	}
	if tt.HasText(i18n.TrafficRuleRedirect()) {
		t.Error("the mock form shows a rewrite field")
	}
	if err := tt.Click(ruleLabel(f.rules[2])); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText(i18n.TrafficRuleDelayHint()) {
		t.Errorf("the delay form is missing its field: %q", tt.Texts())
	}
	if tt.HasText(strings.ToUpper(i18n.TrafficRuleReqHeaders())) || tt.HasText(i18n.TrafficRuleRedirect()) {
		t.Error("the delay form shows a rewrite field")
	}
}

// TestRuleValidation: a field that cannot be used says so at once.
func TestRuleValidation(t *testing.T) {
	f := newTestTraffic()
	f.rules = sampleRules()
	tt := openRules(t, f)
	clickField(t, tt, i18n.StatusLabel())
	tt.Type("99")
	tt.Frame()
	if !tt.HasText(i18n.TrafficRuleStatusBad()) {
		t.Errorf("no status error: %q", tt.Texts())
	}
	if !tt.HasText(i18n.TrafficRuleDelayHint()) && !tt.HasText(i18n.TrafficRuleRegexp()) {
		t.Errorf("the form stopped showing its fields: %q", tt.Texts())
	}
}

// TestRulePrefill: the rules a flow becomes carry what the flow carried.
func TestRulePrefill(t *testing.T) {
	flow := tool.Flow{
		FlowRow: tool.FlowRow{
			Method: "GET", Host: "api.example.com", Path: "/v1/items", Status: 201,
			Type: "application/json",
		},
		URL: "https://api.example.com/v1/items?page=2",
		Response: tool.HTTPMessage{
			Headers: []tool.Header{
				{Name: "Content-Type", Value: "application/json"},
				{Name: "Content-Length", Value: "18"},
				{Name: "Transfer-Encoding", Value: "chunked"},
				{Name: "Set-Cookie", Value: "a=1"},
			},
			Body: tool.Body{Type: "application/json", Data: []byte(`{"ok":true}`)},
		},
	}
	tests := []struct {
		name    string
		build   func(tool.Flow) tool.Rule
		kind    tool.RuleKind
		pattern string
		methods []string
	}{
		{"mock", ruleForMock, tool.RuleMock, "https://api.example.com/v1/items?page=2", []string{"GET"}},
		{"block url", ruleForBlockURL, tool.RuleBlock, "https://api.example.com/v1/items?page=2", []string{"GET"}},
		{"block host", ruleForBlockHost, tool.RuleBlock, "*://api.example.com/*", nil},
		{"rewrite", ruleForRewrite, tool.RuleRewrite, "https://api.example.com/v1/items?page=2", []string{"GET"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkPrefill(t, tt.build(flow), tt.kind, tt.pattern, tt.methods)
		})
	}
	mock := ruleForMock(flow)
	if mock.Mock.Status != 201 {
		t.Errorf("mock status %d, want the flow's 201", mock.Mock.Status)
	}
	if mock.Mock.Body != `{"ok":true}` {
		t.Errorf("mock body %q, want the response's text", mock.Mock.Body)
	}
	for _, h := range mock.Mock.Headers {
		if h.Name == "Content-Length" || h.Name == "Transfer-Encoding" {
			t.Errorf("the mock kept %q, which the connection decides", h.Name)
		}
	}
	if len(mock.Mock.Headers) != 2 {
		t.Errorf("mock headers %v, want the two a server sends", mock.Mock.Headers)
	}
	truncated := flow
	truncated.Response.Body.Truncated = true
	if got := ruleForMock(truncated).Mock.Body; got != "" {
		t.Errorf("a truncated body was mocked as %q", got)
	}
}

// checkPrefill checks one rule a flow became.
func checkPrefill(t *testing.T, got tool.Rule, kind tool.RuleKind, pattern string, methods []string) {
	t.Helper()
	if got.Kind != kind || got.Match.Pattern != pattern {
		t.Errorf("rule %+v, want kind %v and pattern %q", got, kind, pattern)
	}
	if len(got.Match.Methods) != len(methods) {
		t.Errorf("methods %v, want %v", got.Match.Methods, methods)
	}
	if !got.Enabled {
		t.Error("a rule from a flow is not enabled")
	}
}

// TestRuleSetErrorRestores: a book the engine refuses puts the last one
// back, and says why in the engine's words.
func TestRuleSetErrorRestores(t *testing.T) {
	f := newTestTraffic()
	f.rules = sampleRules()
	f.rulesErr = errors.New("rule 1 (rule-2): status 42 is not 200 to 599")
	tt := openRules(t, f)
	if err := tt.Click(i18n.TrafficRuleMoveDown()); err != nil {
		t.Fatal(err)
	}
	waitText(t, tt, i18n.TrafficRuleNotSaved())
	if !tt.HasText("status 42 is not 200 to 599") {
		t.Errorf("the engine's words are not shown: %q", tt.Texts())
	}
	first, okA := tt.Find(ruleLabel(f.rules[0]))
	last, okB := tt.Find(ruleLabel(f.rules[2]))
	if !okA || !okB || first.Y >= last.Y {
		t.Errorf("the list did not go back to its order: %v, %v", first, last)
	}
}
