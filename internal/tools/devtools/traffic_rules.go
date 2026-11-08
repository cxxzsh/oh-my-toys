package devtools

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// The bounds the made-up tool checks a rule against, as the real engine
// does: a status a server can answer, and a delay of at most five minutes.
const (
	ruleStatusMin = 200
	ruleStatusMax = 599
	ruleDelayMax  = 5 * time.Minute
)

// copyRules copies a rule book, so that neither side changes the other's.
func copyRules(in []tool.Rule) []tool.Rule {
	out := make([]tool.Rule, len(in))
	for i, r := range in {
		out[i] = r
		out[i].Match.Methods = append([]string(nil), r.Match.Methods...)
		out[i].Rewrite.RequestHeaders = append([]tool.HeaderEdit(nil), r.Rewrite.RequestHeaders...)
		out[i].Rewrite.ResponseHeaders = append([]tool.HeaderEdit(nil), r.Rewrite.ResponseHeaders...)
		out[i].Mock.Headers = append([]tool.Header(nil), r.Mock.Headers...)
		out[i].Rewrite.RequestBody = copyBodyEdit(r.Rewrite.RequestBody)
		out[i].Rewrite.ResponseBody = copyBodyEdit(r.Rewrite.ResponseBody)
	}
	return out
}

// copyBodyEdit copies a body edit, or nothing.
func copyBodyEdit(b *tool.BodyEdit) *tool.BodyEdit {
	if b == nil {
		return nil
	}
	cp := *b
	return &cp
}

// checkRules refuses a book the real engine would refuse, with errors that
// read the same way.
func checkRules(rules []tool.Rule) error {
	seen := make(map[string]bool, len(rules))
	for i, r := range rules {
		if err := checkRule(i, r, seen); err != nil {
			return err
		}
	}
	return nil
}

// checkRule refuses one rule: its id, what it matches, and what it does.
func checkRule(i int, r tool.Rule, seen map[string]bool) error {
	name := fmt.Sprintf("rule %d", i+1)
	if r.ID == "" {
		return fmt.Errorf("%s: id is empty", name)
	}
	if seen[r.ID] {
		return fmt.Errorf("duplicate rule id %q", r.ID)
	}
	seen[r.ID] = true
	if err := checkMatch(name, r); err != nil {
		return err
	}
	if err := checkEffect(name, r); err != nil {
		return err
	}
	return nil
}

// checkMatch refuses a pattern that is empty or does not compile.
func checkMatch(name string, r tool.Rule) error {
	if strings.TrimSpace(r.Match.Pattern) == "" {
		return fmt.Errorf("%s (%s): pattern is empty", name, r.ID)
	}
	if r.Match.Regexp {
		if _, err := regexp.Compile(r.Match.Pattern); err != nil {
			return fmt.Errorf("%s (%s): pattern does not compile: %w", name, r.ID, err)
		}
	}
	return nil
}

// checkEffect refuses a status, header or delay the engine would not take.
func checkEffect(name string, r tool.Rule) error {
	if err := checkStatus(name, r); err != nil {
		return err
	}
	if err := checkHeaders(name, r); err != nil {
		return err
	}
	if r.Delay < 0 || r.Delay > ruleDelayMax {
		return fmt.Errorf("%s (%s): delay %s is not 0 to %s", name, r.ID, r.Delay, ruleDelayMax)
	}
	return nil
}

// checkStatus refuses a status a server cannot answer.
func checkStatus(name string, r tool.Rule) error {
	for _, status := range []int{r.Mock.Status, r.Rewrite.Status} {
		if status != 0 && (status < ruleStatusMin || status > ruleStatusMax) {
			return fmt.Errorf("%s (%s): status %d is not %d to %d", name, r.ID, status, ruleStatusMin, ruleStatusMax)
		}
	}
	return nil
}

// checkHeaders refuses a header name or value with a line break in it.
func checkHeaders(name string, r tool.Rule) error {
	edits := append(append([]tool.HeaderEdit(nil), r.Rewrite.RequestHeaders...), r.Rewrite.ResponseHeaders...)
	for _, h := range edits {
		if strings.ContainsAny(h.Name, "\r\n") || strings.ContainsAny(h.Value, "\r\n") {
			return fmt.Errorf("%s (%s): header %q has a line break", name, r.ID, h.Name)
		}
	}
	for _, h := range r.Mock.Headers {
		if strings.ContainsAny(h.Name, "\r\n") || strings.ContainsAny(h.Value, "\r\n") {
			return fmt.Errorf("%s (%s): header %q has a line break", name, r.ID, h.Name)
		}
	}
	return nil
}
