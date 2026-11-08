package rules

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// matcher decides whether a request's URL matches a rule.
type matcher struct {
	// parts is the pattern split at "*", so a glob is not re-split on every
	// request; it is nil when the rule reads the pattern as a regular
	// expression.
	parts []string
	re    *regexp.Regexp
	// pathOnly is set when the pattern has no "://": such a pattern is
	// matched against host/path?query, whatever the scheme, which is what a
	// person writing "example.com/api/*" expects.
	pathOnly bool
}

// compileMatcher prepares a rule's pattern once.
func compileMatcher(i int, r tool.Rule) (matcher, error) {
	m := matcher{pathOnly: !strings.Contains(r.Match.Pattern, "://")}
	if !r.Match.Regexp {
		m.parts = strings.Split(r.Match.Pattern, "*")
		return m, nil
	}
	re, err := regexp.Compile(r.Match.Pattern)
	if err != nil {
		return matcher{}, invalidRule(i, r, "pattern does not compile: "+err.Error())
	}
	m.re = re
	return m, nil
}

// appliesTo reports whether the rule is meant for req.
func (r rule) appliesTo(req *http.Request) bool {
	return r.methodMatches(req.Method) && r.match.matches(req)
}

// methodMatches reports whether req's method is one the rule lists; an
// empty list matches any.
func (r rule) methodMatches(method string) bool {
	methods := r.raw.Match.Methods
	if len(methods) == 0 {
		return true
	}
	for _, m := range methods {
		if strings.EqualFold(m, method) {
			return true
		}
	}
	return false
}

// matches reports whether the pattern matches the request's URL.
func (m matcher) matches(req *http.Request) bool {
	if req.URL == nil {
		return false
	}
	host := req.URL.Host
	if host == "" {
		host = req.Host
	}
	target := host + req.URL.RequestURI()
	if !m.pathOnly {
		target = req.URL.Scheme + "://" + target
	}
	if m.re != nil {
		return m.re.MatchString(target)
	}
	return matchGlob(m.parts, target)
}

// matchGlob matches s against a pattern split at "*": the parts must appear
// in turn, the first at the start and the last at the end, so a glob is
// anchored at both ends, and "*" may cross "/".
func matchGlob(parts []string, s string) bool {
	if !strings.HasPrefix(s, parts[0]) {
		return false
	}
	s = s[len(parts[0]):]
	last := len(parts) - 1
	if last == 0 {
		return s == ""
	}
	for _, part := range parts[1:last] {
		if part == "" {
			continue
		}
		i := strings.Index(s, part)
		if i < 0 {
			return false
		}
		s = s[i+len(part):]
	}
	return strings.HasSuffix(s, parts[last])
}
