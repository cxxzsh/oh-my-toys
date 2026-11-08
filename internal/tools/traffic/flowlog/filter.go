package flowlog

import (
	"net"
	"strings"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// statusClassDivisor is the hundred whose quotient is the status class:
// 2 for every 2xx, 0 for no response.
const statusClassDivisor = 100

// criteria is a filter ready for matching many flows: the collections kept
// in the case they are compared in, so the work is done once per Rows.
type criteria struct {
	text    string
	hosts   []string
	methods []string
	types   []string
	classes []int
	source  tool.FlowSource
	failed  bool
}

func newCriteria(f tool.Filter) *criteria {
	return &criteria{
		text:    strings.ToLower(f.Text),
		hosts:   lowerAll(f.Hosts),
		methods: lowerAll(f.Methods),
		types:   lowerAll(f.Types),
		classes: f.StatusClasses,
		source:  f.Source,
		failed:  f.Failed,
	}
}

func lowerAll(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = strings.ToLower(s)
	}
	return out
}

// matches reports whether fl passes every field of the filter.
func (c *criteria) matches(fl *tool.Flow) bool {
	if fl.Source != c.source {
		return false
	}
	if c.failed && fl.State != tool.FlowFailed && fl.State != tool.FlowBlocked {
		return false
	}
	if len(c.methods) > 0 && !anyEqualFold(c.methods, fl.Method) {
		return false
	}
	if len(c.classes) > 0 && !anyClass(c.classes, fl.Status) {
		return false
	}
	if len(c.types) > 0 && !anyPrefix(c.types, fl.Type) {
		return false
	}
	if len(c.hosts) > 0 && !c.hostMatches(fl.Host) {
		return false
	}
	return c.textMatches(fl)
}

func anyEqualFold(list []string, s string) bool {
	for _, want := range list {
		if strings.EqualFold(want, s) {
			return true
		}
	}
	return false
}

func anyClass(classes []int, status int) bool {
	class := 0
	if status > 0 {
		class = status / statusClassDivisor
	}
	for _, want := range classes {
		if want == class {
			return true
		}
	}
	return false
}

func anyPrefix(prefixes []string, s string) bool {
	s = strings.ToLower(s)
	for _, prefix := range prefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

// hostMatches matches the host of a flow, with or without its port, against
// the host patterns of the filter.
func (c *criteria) hostMatches(host string) bool {
	if host == "" {
		return false
	}
	names := []string{strings.ToLower(host)}
	if h, _, err := net.SplitHostPort(host); err == nil {
		names = append(names, strings.ToLower(h))
	}
	for _, pattern := range c.hosts {
		for _, name := range names {
			if matchGlob(pattern, name) {
				return true
			}
		}
	}
	return false
}

// matchGlob reports whether s matches pattern, where "*" stands for any run
// of characters. Both are already lower case.
func matchGlob(pattern, s string) bool {
	p, i := 0, 0
	star, mark := -1, 0
	for i < len(s) {
		switch {
		case p < len(pattern) && pattern[p] == '*':
			star = p
			mark = i
			p++
		case p < len(pattern) && pattern[p] == s[i]:
			p++
			i++
		case star >= 0:
			p = star + 1
			mark++
			i = mark
		default:
			return false
		}
	}
	for p < len(pattern) && pattern[p] == '*' {
		p++
	}
	return p == len(pattern)
}

// textMatches reports whether the text of the filter appears, ignoring
// case, in the URL, a header, a text body or the note of the flow.
func (c *criteria) textMatches(fl *tool.Flow) bool {
	if c.text == "" {
		return true
	}
	if containsFoldString(fl.URL, c.text) || containsFoldString(fl.Info, c.text) {
		return true
	}
	return messageMatches(&fl.Request, c.text) || messageMatches(&fl.Response, c.text)
}

func messageMatches(m *tool.HTTPMessage, needle string) bool {
	for _, h := range m.Headers {
		if containsFoldString(h.Name, needle) || containsFoldString(h.Value, needle) {
			return true
		}
	}
	return isTextType(m.Body.Type) && containsFold(m.Body.Data, needle)
}

// containsFoldString reports whether s contains needle, already lower case,
// ignoring case. It is for the short strings of a flow.
func containsFoldString(s, needle string) bool {
	return strings.Contains(strings.ToLower(s), needle)
}

// containsFold reports whether data contains needle, already lower case,
// comparing ASCII letters without regard to case. Bodies are searched
// without copying them, which a text filter makes worth the loop; letters
// outside ASCII must match exactly, which text without case does.
func containsFold(data []byte, needle string) bool {
	if needle == "" {
		return false
	}
	n := len(needle)
	for i := 0; i+n <= len(data); i++ {
		if lowerASCII(data[i]) == needle[0] && equalFoldAt(data[i:], needle) {
			return true
		}
	}
	return false
}

func equalFoldAt(data []byte, needle string) bool {
	for j := range len(needle) {
		if lowerASCII(data[j]) != needle[j] {
			return false
		}
	}
	return true
}

func lowerASCII(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}

// isTextType reports whether a body of media type t holds text, so that the
// inspector may search it; the bytes of other types are not text.
func isTextType(t string) bool {
	t = strings.ToLower(t)
	if i := strings.IndexByte(t, ';'); i >= 0 {
		t = strings.TrimSpace(t[:i])
	}
	if strings.HasPrefix(t, "text/") || strings.HasSuffix(t, "+json") || strings.HasSuffix(t, "+xml") {
		return true
	}
	switch t {
	case "application/json", "application/xml",
		"application/x-www-form-urlencoded", "application/javascript":
		return true
	}
	return false
}
