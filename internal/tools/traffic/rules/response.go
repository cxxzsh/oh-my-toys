package rules

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// Response applies the rewrite rules to a response in place and returns
// the names of the rules that applied. A rule whose body edit could not be
// done is reported in the error and left out of the names.
func (e *Engine) Response(req *http.Request, resp *http.Response) ([]string, error) {
	b := e.current.Load()
	if b == nil || req == nil || resp == nil {
		return nil, nil
	}
	var (
		hits []string
		errs []error
		seen = make(map[string]struct{})
	)
	for _, r := range b.rules {
		if !r.raw.Enabled || r.raw.Kind != tool.RuleRewrite || !r.appliesTo(req) {
			continue
		}
		if err := r.applyResponse(resp); err != nil {
			errs = append(errs, err)
			continue
		}
		addHit(&hits, seen, r.label())
	}
	return hits, errors.Join(errs...)
}

// applyResponse changes the response as the rule says. The body goes first:
// its edit needs the encoding header that the header edits may remove.
func (r rule) applyResponse(resp *http.Response) error {
	if resp.Header == nil {
		resp.Header = make(http.Header)
	}
	if r.rewrite.respBody != nil {
		if err := editResponseBody(resp, r.rewrite.respBody); err != nil {
			return r.notApplied(err)
		}
	}
	editHeaders(resp.Header, r.raw.Rewrite.ResponseHeaders)
	if status := r.raw.Rewrite.Status; status != 0 {
		setStatus(resp, status)
	}
	return nil
}

// statusLine is the status line for a code, such as "200 OK".
func statusLine(code int) string {
	if text := http.StatusText(code); text != "" {
		return strconv.Itoa(code) + " " + text
	}
	return strconv.Itoa(code)
}

// setStatus replaces the status of resp.
func setStatus(resp *http.Response, code int) {
	resp.StatusCode = code
	resp.Status = statusLine(code)
}
