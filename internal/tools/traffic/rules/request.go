package rules

import (
	"errors"
	"net/http"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// Request applies the rule book to an outgoing request, changing req in
// place. The verdict says what to do with the request: answer it, close
// the connection, hold the response back. Rewrites apply in book order,
// the first mock or block ends the book, and every delay adds up.
//
// A rule Request could not apply (a body over tool.MaxBody, a mock file it
// cannot read) is left out of the verdict and reported in the error; the
// rest of the book still applies.
func (e *Engine) Request(req *http.Request) (Verdict, error) {
	b := e.current.Load()
	if b == nil || req == nil {
		return Verdict{}, nil
	}
	var (
		v    Verdict
		errs []error
		seen = make(map[string]struct{})
	)
	for _, r := range b.rules {
		if !r.raw.Enabled || !r.appliesTo(req) {
			continue
		}
		switch r.raw.Kind {
		case tool.RuleRewrite:
			if err := r.applyRequest(req); err != nil {
				errs = append(errs, err)
				continue
			}
			addHit(&v.Hits, seen, r.label())
		case tool.RuleMock:
			resp, err := r.mock(req) //nolint:bodyclose // the verdict hands the body to the proxy
			if err != nil {
				errs = append(errs, err)
				continue
			}
			v.Mock = resp
			addHit(&v.Hits, seen, r.label())
			return v, errors.Join(errs...)
		case tool.RuleBlock:
			v.Block = true
			addHit(&v.Hits, seen, r.label())
			return v, errors.Join(errs...)
		case tool.RuleDelay:
			v.Delay += r.raw.Delay
			addHit(&v.Hits, seen, r.label())
		}
	}
	return v, errors.Join(errs...)
}

// applyRequest changes req as the rule says. It gives up before changing
// anything when the request body cannot be taken whole, so a skipped rule
// leaves the request exactly as the last applied rule left it.
func (r rule) applyRequest(req *http.Request) error {
	var body []byte
	if r.rewrite.reqBody != nil {
		data, err := readBody(&req.Body)
		if err != nil {
			return r.notApplied(err)
		}
		body = r.rewrite.reqBody.apply(data)
	}
	r.redirect(req)
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	editRequestHeaders(req, r.raw.Rewrite.RequestHeaders)
	if r.rewrite.reqBody != nil {
		setRequestBody(req, body)
	}
	return nil
}

// redirect points req at the rule's URL. The compiled URL is shared by
// every request the rule applies to, so each request gets its own copy.
func (r rule) redirect(req *http.Request) {
	if r.rewrite.url == nil {
		return
	}
	u := *r.rewrite.url
	req.URL = &u
	req.Host = u.Host
}

// addHit records a rule's label once, however often the rule applied.
func addHit(hits *[]string, seen map[string]struct{}, label string) {
	if _, ok := seen[label]; ok {
		return
	}
	seen[label] = struct{}{}
	*hits = append(*hits, label)
}
