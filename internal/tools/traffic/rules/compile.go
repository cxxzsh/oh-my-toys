package rules

import (
	"fmt"
	"net/url"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// rule is one compiled rule: the rule as it was set, plus the parts that
// are cheaper to work out once than on every exchange.
type rule struct {
	raw     tool.Rule
	match   matcher
	rewrite rewrite
}

// rewrite is a RewriteEffect with its URL parsed and its body edits
// compiled.
type rewrite struct {
	url      *url.URL
	reqBody  *bodyEdit
	respBody *bodyEdit
}

// compileBook validates rules and compiles them into an immutable book.
func compileBook(rules []tool.Rule) (*book, error) {
	b := &book{rules: make([]rule, 0, len(rules))}
	ids := make(map[string]struct{}, len(rules))
	for i, r := range rules {
		if r.ID == "" {
			return nil, invalidRule(i, r, "needs an ID")
		}
		if _, dup := ids[r.ID]; dup {
			return nil, invalidRule(i, r, fmt.Sprintf("repeats the ID %q", r.ID))
		}
		ids[r.ID] = struct{}{}
		c, err := compileRule(i, r)
		if err != nil {
			return nil, err
		}
		b.rules = append(b.rules, c)
	}
	return b, nil
}

// compileRule checks one rule and compiles its parts.
func compileRule(i int, r tool.Rule) (rule, error) {
	if r.Match.Pattern == "" {
		return rule{}, invalidRule(i, r, "needs a pattern")
	}
	if r.Delay < 0 || r.Delay > maxDelay {
		return rule{}, invalidRule(i, r,
			fmt.Sprintf("delay %v is not between 0 and %v", r.Delay, maxDelay))
	}
	if err := checkStatus(i, r, r.Mock.Status); err != nil {
		return rule{}, err
	}
	if err := checkStatus(i, r, r.Rewrite.Status); err != nil {
		return rule{}, err
	}
	if err := checkHeaderEdits(i, r, r.Rewrite.RequestHeaders); err != nil {
		return rule{}, err
	}
	if err := checkHeaderEdits(i, r, r.Rewrite.ResponseHeaders); err != nil {
		return rule{}, err
	}
	if err := checkMockHeaders(i, r, r.Mock.Headers); err != nil {
		return rule{}, err
	}
	m, err := compileMatcher(i, r)
	if err != nil {
		return rule{}, err
	}
	rw, err := compileRewrite(i, r)
	if err != nil {
		return rule{}, err
	}
	return rule{raw: copyRule(r), match: m, rewrite: rw}, nil
}

// compileRewrite parses what a rewrite asks for once, so that applying it
// cannot fail after the request body has been read.
func compileRewrite(i int, r tool.Rule) (rewrite, error) {
	var rw rewrite
	if r.Rewrite.URL != "" {
		u, err := url.Parse(r.Rewrite.URL)
		if err != nil {
			return rewrite{}, invalidRule(i, r, fmt.Sprintf("URL %q does not parse: %v", r.Rewrite.URL, err))
		}
		rw.url = u
	}
	reqBody, err := compileBodyEdit(i, r, r.Rewrite.RequestBody)
	if err != nil {
		return rewrite{}, err
	}
	respBody, err := compileBodyEdit(i, r, r.Rewrite.ResponseBody)
	if err != nil {
		return rewrite{}, err
	}
	rw.reqBody, rw.respBody = reqBody, respBody
	return rw, nil
}

// label is the rule's name, or its ID when it has none.
func (r rule) label() string {
	if r.raw.Name != "" {
		return r.raw.Name
	}
	return r.raw.ID
}

// notApplied names the rule in an error about a part it could not do.
func (r rule) notApplied(err error) error {
	return fmt.Errorf("%s: %w", r.label(), err)
}

// copyRule deep-copies a rule, so the engine neither keeps nor hands out a
// slice the caller can change.
func copyRule(r tool.Rule) tool.Rule {
	c := r
	c.Match.Methods = append([]string(nil), r.Match.Methods...)
	c.Rewrite.RequestHeaders = append([]tool.HeaderEdit(nil), r.Rewrite.RequestHeaders...)
	c.Rewrite.ResponseHeaders = append([]tool.HeaderEdit(nil), r.Rewrite.ResponseHeaders...)
	c.Rewrite.RequestBody = copyBodyEdit(r.Rewrite.RequestBody)
	c.Rewrite.ResponseBody = copyBodyEdit(r.Rewrite.ResponseBody)
	c.Mock.Headers = append([]tool.Header(nil), r.Mock.Headers...)
	return c
}

// copyBodyEdit copies a body edit; nil stays nil.
func copyBodyEdit(b *tool.BodyEdit) *tool.BodyEdit {
	if b == nil {
		return nil
	}
	c := *b
	return &c
}
