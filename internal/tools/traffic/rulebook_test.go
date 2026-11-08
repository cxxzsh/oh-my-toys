package traffic

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// mockRule answers every request for example.test with a response of its
// own.
func mockRule() tool.Rule {
	return tool.Rule{
		ID: "mock-1", Name: "Mock example", Enabled: true,
		Match: tool.RuleMatch{Pattern: "http://example.test/*"},
		Kind:  tool.RuleMock,
		Mock: tool.MockEffect{
			Status:  201,
			Body:    "mocked",
			Headers: []tool.Header{{Name: "Content-Type", Value: "text/plain"}},
		},
	}
}

// rewriteRule changes the response status of every request.
func rewriteRule() tool.Rule {
	return tool.Rule{
		ID: "rewrite-1", Name: "Rewrite", Enabled: true,
		Match: tool.RuleMatch{Pattern: "*"},
		Kind:  tool.RuleRewrite,
		Rewrite: tool.RewriteEffect{
			ResponseHeaders: []tool.HeaderEdit{{Name: "X-Test", Value: "yes"}},
		},
	}
}

func TestSetRulesKeepsTheBookForTheNextLaunch(t *testing.T) {
	tl, _ := testTool(t)
	book := []tool.Rule{mockRule(), rewriteRule()}
	if err := tl.SetRules(book); err != nil {
		t.Fatalf("SetRules: %v", err)
	}
	if got := tl.Rules(); len(got) != 2 || got[0].ID != "mock-1" {
		t.Fatalf("Rules() = %+v, want the book just set", got)
	}
	checkMode(t, tl.path(rulesFile), fileMode)

	again := newTool(filepath.Dir(tl.dir), (&fakeSystem{}).system())
	if got := again.Rules(); len(got) != 2 || got[1].ID != "rewrite-1" {
		t.Errorf("Rules() after a restart = %+v, want the saved book", got)
	}
}

func TestSetRulesRejectsWhatTheEngineRejects(t *testing.T) {
	tl, _ := testTool(t)
	if err := tl.SetRules([]tool.Rule{rewriteRule()}); err != nil {
		t.Fatalf("SetRules: %v", err)
	}
	bad := []tool.Rule{{
		ID: "bad", Enabled: true,
		Match: tool.RuleMatch{Pattern: "[", Regexp: true},
		Kind:  tool.RuleBlock,
	}}
	if err := tl.SetRules(bad); err == nil {
		t.Fatal("SetRules of a pattern that does not compile: no error")
	}
	if got := tl.Rules(); len(got) != 1 || got[0].ID != "rewrite-1" {
		t.Errorf("Rules() = %+v, want the book in use kept", got)
	}
	if _, err := os.Stat(tl.path(rulesFile)); err != nil {
		t.Errorf("stat %s: %v, want the book that worked kept", rulesFile, err)
	}
}

func TestRulesFileDamageLeavesNoRules(t *testing.T) {
	tl, _ := testTool(t)
	if err := os.MkdirAll(tl.dir, folderMode); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(tl.path(rulesFile), []byte("{ nope"), fileMode); err != nil {
		t.Fatalf("write: %v", err)
	}
	again := newTool(filepath.Dir(tl.dir), (&fakeSystem{}).system())
	if got := again.Rules(); len(got) != 0 {
		t.Errorf("Rules() = %+v, want none: the file is damaged", got)
	}
}

func TestARuleMocksARequestThroughTheCapture(t *testing.T) {
	tl, _ := testTool(t)
	cfg := proxyConfig(t, false)
	if err := tl.SetConfig(cfg); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if err := tl.SetRules([]tool.Rule{mockRule()}); err != nil {
		t.Fatalf("SetRules: %v", err)
	}
	if err := tl.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = tl.Stop() })

	if body := getThroughCapture(t, cfg.ProxyPort, "http://example.test/anything"); body != "mocked" {
		t.Errorf("body %q, want the answer of the rule", body)
	}
	rows := waitRows(t, tl.Log(), tool.SourceProxy, 1)
	flow, ok := tl.Log().Get(rows[0].ID)
	if !ok {
		t.Fatalf("flow %d is gone", rows[0].ID)
	}
	if flow.Status != 201 {
		t.Errorf("status %d, want 201", flow.Status)
	}
	if !flow.Ruled || len(flow.RuleHits) == 0 {
		t.Errorf("flow %+v, want it marked as ruled and naming the rule", flow.FlowRow)
	}
}

// TestSetRulesUnderLoad: the rule editor saves on every keystroke, so two
// saves must not share one scratch file or one engine swap.
func TestSetRulesUnderLoad(t *testing.T) {
	tl, _ := testTool(t)
	book := []tool.Rule{mockRule(), rewriteRule()}
	const writers, each = 4, 50
	errs := make(chan error, writers*each)
	var wg sync.WaitGroup
	for range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range each {
				errs <- tl.SetRules(book)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("SetRules: %v", err)
		}
	}
	if got := tl.Rules(); len(got) != 2 {
		t.Errorf("Rules() = %+v, want the book just set", got)
	}
	var saved []tool.Rule
	if err := readJSON(tl.path(rulesFile), &saved); err != nil {
		t.Fatalf("the saved book does not parse: %v", err)
	}
	if len(saved) != 2 {
		t.Errorf("the saved book holds %d rules, want 2", len(saved))
	}
}

// TestRulesFileTheEngineRejectsIsMovedAside: a book the engine refuses is
// kept for a look instead of being silently overwritten by the next save.
func TestRulesFileTheEngineRejectsIsMovedAside(t *testing.T) {
	tl, _ := testTool(t)
	if err := os.MkdirAll(tl.dir, folderMode); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	bad := []tool.Rule{{
		ID: "bad", Enabled: true,
		Match: tool.RuleMatch{Pattern: "[", Regexp: true},
		Kind:  tool.RuleBlock,
	}}
	if err := writeJSON(tl.path(rulesFile), bad); err != nil {
		t.Fatalf("write: %v", err)
	}
	again := newTool(filepath.Dir(tl.dir), (&fakeSystem{}).system())
	if got := again.Rules(); len(got) != 0 {
		t.Errorf("Rules() = %+v, want none: the engine refuses the file", got)
	}
	if _, err := os.Stat(again.path(rulesFile) + brokenSuffix); err != nil {
		t.Errorf("stat %s%s: %v, want the refused book kept", rulesFile, brokenSuffix, err)
	}
}
