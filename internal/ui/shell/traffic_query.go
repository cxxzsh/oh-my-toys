package shell

import (
	"net"
	"slices"
	"strconv"
	"strings"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// The keys the search field reads as "key:value". A word with any other key
// is plain text, as is a word whose value the key does not take.
const (
	queryHost    = "host"
	queryMethod  = "method"
	queryStatus  = "status"
	queryProgram = "program"
	queryType    = "type"
	queryFailed  = "failed"
)

// The status classes and codes a query can name. The log's status classes
// are the hundreds; a flow with no response has class 0.
const (
	statusNoResponse  = 0
	statusClassWidth  = 100
	statusMin         = 100
	statusMax         = 599
	queryNoneWord     = "none"
	queryStatusClassN = 2 // the length of a class like "4xx" after its digit
)

// queryTypeNames are the words a type: token picks, by media type chip.
var queryTypeNames = [trafficTypeCount]string{
	typeJSON:     "json",
	typeForm:     "form",
	typeXML:      "xml",
	typeJS:       "js",
	typeCSS:      "css",
	typeDocument: "html",
	typeMedia:    "image",
}

// queryToken is a piece of the search text that reads as a key and a value.
// Its span locates it in the text, so its chip can remove it.
type queryToken struct {
	raw        string // the piece, with its quotes dropped
	key        string // the lower-case key, for a token
	value      string // the value as typed, for a token
	start, end int    // the byte span of the piece in the text
}

// queryTerms is what the search text asks for. Each part narrows the rows
// in narrowRows, and all parts must hold together.
type queryTerms struct {
	words    []string // plain words, lower case: every one must appear in a flow
	hosts    []string // host patterns, where "*" stands for any run of characters
	methods  []string // upper case
	programs []string // program names, compared without case
	types    []int    // media type chips, as typeJSON and the rest
	classes  []int    // status classes, 0 for no response
	codes    []int    // exact status codes
	failed   bool
	tokens   []queryToken // the recognised key:value pieces, in order
}

// parseQuery reads the search text. It never fails: a piece that is not a
// valid key:value token is a plain word.
func parseQuery(text string) queryTerms {
	var q queryTerms
	for _, piece := range splitQuery(text) {
		if key, value, ok := cutQueryKey(piece.raw); ok && q.apply(key, value) {
			piece.key, piece.value = key, value
			q.tokens = append(q.tokens, piece)
			continue
		}
		if piece.raw != "" {
			q.words = append(q.words, strings.ToLower(piece.raw))
		}
	}
	return q
}

// splitQuery cuts the text at white space. A double quote keeps the spaces
// of its run and is dropped, so program:"Code Helper" is one piece. An
// unclosed quote runs to the end of the text.
func splitQuery(text string) []queryToken {
	var out []queryToken
	for i := 0; i < len(text); {
		if isQuerySpace(text[i]) {
			i++
			continue
		}
		start := i
		var raw strings.Builder
		for i < len(text) && !isQuerySpace(text[i]) {
			if text[i] == '"' {
				i++
				for i < len(text) && text[i] != '"' {
					raw.WriteByte(text[i])
					i++
				}
				i++ // the closing quote, or the end when there is none
				continue
			}
			raw.WriteByte(text[i])
			i++
		}
		out = append(out, queryToken{raw: raw.String(), start: start, end: min(i, len(text))})
	}
	return out
}

func isQuerySpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

// cutQueryKey splits a piece at its first colon when the key before it is one
// the search field knows. Keys are case-insensitive.
func cutQueryKey(raw string) (key, value string, ok bool) {
	k, v, found := strings.Cut(raw, ":")
	if !found || v == "" {
		return "", "", false
	}
	k = strings.ToLower(k)
	switch k {
	case queryHost, queryMethod, queryStatus, queryProgram, queryType, queryFailed:
		return k, v, true
	}
	return "", "", false
}

// apply records a token and reports whether its value is one the key takes.
// A rejected value records nothing, so the piece stays a plain word.
func (q *queryTerms) apply(key, value string) bool {
	switch key {
	case queryHost:
		q.hosts = append(q.hosts, strings.ToLower(value))
		return true
	case queryProgram:
		q.programs = append(q.programs, value)
		return true
	case queryMethod:
		return q.addMethods(value)
	case queryStatus:
		return q.addStatuses(value)
	case queryType:
		return q.addTypes(value)
	case queryFailed:
		if strings.EqualFold(value, "true") {
			q.failed = true
			return true
		}
	}
	return false
}

// addMethods takes a comma list of method names.
func (q *queryTerms) addMethods(value string) bool {
	var found []string
	for _, m := range strings.Split(value, ",") {
		if m == "" {
			continue
		}
		if !isMethodName(m) {
			return false
		}
		found = append(found, strings.ToUpper(m))
	}
	if len(found) == 0 {
		return false
	}
	q.methods = append(q.methods, found...)
	return true
}

func isMethodName(s string) bool {
	for i := range len(s) {
		c := s[i]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') {
			return false
		}
	}
	return true
}

// addStatuses takes a comma list of statuses: "none", a class such as
// "4xx", or a code such as "404".
func (q *queryTerms) addStatuses(value string) bool {
	var classes, codes []int
	valid := 0
	for _, item := range strings.Split(value, ",") {
		if item == "" {
			continue
		}
		class, code, ok := parseStatusItem(strings.ToLower(item))
		if !ok {
			return false
		}
		valid++
		// A code names one status; only a class or "none" widens to a whole class.
		if code != 0 {
			codes = append(codes, code)
		} else {
			classes = append(classes, class)
		}
	}
	if valid == 0 {
		return false
	}
	q.classes = append(q.classes, classes...)
	q.codes = append(q.codes, codes...)
	return true
}

// parseStatusItem reads one status. For a class it returns the class alone;
// for a code it returns the class of the code and the code.
func parseStatusItem(item string) (class, code int, ok bool) {
	if item == queryNoneWord {
		return statusNoResponse, 0, true
	}
	if len(item) == queryStatusClassN+1 && item[1:] == "xx" && item[0] >= '1' && item[0] <= '5' {
		return int(item[0] - '0'), 0, true
	}
	n, err := strconv.Atoi(item)
	if err != nil || n < statusMin || n > statusMax {
		return 0, 0, false
	}
	return n / statusClassWidth, n, true
}

// addTypes takes a comma list of media type names.
func (q *queryTerms) addTypes(value string) bool {
	var found []int
	for _, name := range strings.Split(value, ",") {
		if name == "" {
			continue
		}
		i, ok := typeNamed(strings.ToLower(name))
		if !ok {
			return false
		}
		found = append(found, i)
	}
	if len(found) == 0 {
		return false
	}
	q.types = append(q.types, found...)
	return true
}

// typeNamed finds the type chip a name in a type: token picks.
func typeNamed(name string) (int, bool) {
	for i, n := range queryTypeNames {
		if n != "" && n == name {
			return i, true
		}
	}
	return 0, false
}

// logWord is the word the log searches for: the longest, which the most
// flows lack. The other words are checked on the rows, in wordsMatch.
func (q queryTerms) logWord() string {
	best := ""
	for _, w := range q.words {
		if len(w) > len(best) {
			best = w
		}
	}
	return best
}

// rowMatches reports whether a row passes every token of the query. The
// log cannot take these, so the rows are narrowed here.
func (q queryTerms) rowMatches(r tool.FlowRow) bool {
	return allowed(q.methods, strings.ToUpper(r.Method)) &&
		anyFold(q.programs, r.Process.Name) &&
		hostAllowed(q.hosts, r.Host) &&
		q.typeAllowed(r.Type) &&
		q.statusAllowed(r.Status) &&
		(!q.failed || r.State == tool.FlowFailed || r.State == tool.FlowBlocked)
}

// allowed reports whether got is in list. An empty list allows everything.
func allowed[T comparable](list []T, got T) bool {
	return len(list) == 0 || slices.Contains(list, got)
}

// anyFold reports whether s equals one of list, without case. An empty list
// allows everything.
func anyFold(list []string, s string) bool {
	if len(list) == 0 {
		return true
	}
	for _, want := range list {
		if strings.EqualFold(want, s) {
			return true
		}
	}
	return false
}

// hostAllowed matches the host, with or without its port, against the host
// patterns. A row with no host matches no pattern.
func hostAllowed(patterns []string, host string) bool {
	if len(patterns) == 0 {
		return true
	}
	if host == "" {
		return false
	}
	names := []string{strings.ToLower(host)}
	if h, _, err := net.SplitHostPort(host); err == nil {
		names = append(names, strings.ToLower(h))
	}
	for _, p := range patterns {
		for _, n := range names {
			if globMatch(p, n) {
				return true
			}
		}
	}
	return false
}

// globMatch reports whether s matches pattern, where "*" stands for any run
// of characters. Both are already lower case.
func globMatch(pattern, s string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == s
	}
	if !strings.HasPrefix(s, parts[0]) {
		return false
	}
	s = s[len(parts[0]):]
	for _, mid := range parts[1 : len(parts)-1] {
		i := strings.Index(s, mid)
		if i < 0 {
			return false
		}
		s = s[i+len(mid):]
	}
	return strings.HasSuffix(s, parts[len(parts)-1])
}

// typeAllowed reports whether the media type starts with a prefix of a type:
// token. An empty list allows everything.
func (q queryTerms) typeAllowed(media string) bool {
	if len(q.types) == 0 {
		return true
	}
	media = strings.ToLower(media)
	for _, i := range q.types {
		for _, p := range trafficTypePrefixes(i) {
			if strings.HasPrefix(media, p) {
				return true
			}
		}
	}
	return false
}

// statusAllowed reports whether the status is in a class or is a code the
// query names. A query with neither allows everything.
func (q queryTerms) statusAllowed(status int) bool {
	if len(q.classes) == 0 && len(q.codes) == 0 {
		return true
	}
	return slices.Contains(q.classes, status/statusClassWidth) || slices.Contains(q.codes, status)
}

// wordsMatch reports whether every plain word appears in the flow, as the
// log matches its own word: in the URL, the note, a header or a text body.
// The log already matched the longest word, so a single word needs no read.
func (st *trafficState) wordsMatch(words []string, id uint64) bool {
	if len(words) < 2 {
		return true
	}
	fl, ok := st.log.Get(id)
	if !ok {
		return false
	}
	for _, w := range words {
		if !flowHasText(fl, w) {
			return false
		}
	}
	return true
}

// flowHasText reports whether a lower-case needle appears in the flow.
func flowHasText(fl tool.Flow, needle string) bool {
	if strings.Contains(strings.ToLower(fl.URL), needle) || strings.Contains(strings.ToLower(fl.Info), needle) {
		return true
	}
	return messageHasText(fl.Request, needle) || messageHasText(fl.Response, needle)
}

func messageHasText(m tool.HTTPMessage, needle string) bool {
	for _, h := range m.Headers {
		if strings.Contains(strings.ToLower(h.Name), needle) || strings.Contains(strings.ToLower(h.Value), needle) {
			return true
		}
	}
	return isTextMedia(m.Body.Type) && strings.Contains(strings.ToLower(string(m.Body.Data)), needle)
}

// isTextMedia reports whether a body of this media type holds text, which
// the log searches. Other bodies are bytes no word is looked for in.
func isTextMedia(media string) bool {
	media = strings.ToLower(media)
	if i := strings.IndexByte(media, ';'); i >= 0 {
		media = strings.TrimSpace(media[:i])
	}
	if strings.HasPrefix(media, "text/") || strings.HasSuffix(media, "+json") || strings.HasSuffix(media, "+xml") {
		return true
	}
	switch media {
	case "application/json", "application/xml", "application/x-www-form-urlencoded", "application/javascript":
		return true
	}
	return false
}

// withoutToken returns the text with one token's span removed. The pieces on
// either side are joined with one space.
func withoutToken(text string, t queryToken) string {
	return strings.TrimSpace(text[:t.start] + " " + text[t.end:])
}
