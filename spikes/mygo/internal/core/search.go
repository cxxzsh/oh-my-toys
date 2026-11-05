package core

import (
	"slices"
	"strings"
	"unicode"

	"github.com/cxxzsh/oh-my-toys/spikes/mygo/internal/tool"

	"github.com/mozillazg/go-pinyin"

	"github.com/cxxzsh/oh-my-toys/spikes/mygo/internal/i18n"
)

// Languages lists the catalogs the index reads names from, so a tool is
// found by its English and its Chinese name whatever language shows.
func Languages() []string { return []string{"en", "zh-Hans"} }

// Scores of the ways a query can match, lower first.
const (
	scorePrefix       = 0
	scoreContains     = 10
	scoreInitials     = 20
	scoreSpelled      = 25
	scoreInitialsMid  = 30
	scoreSpelledMid   = 35
	scoreKeyword      = 40
	scoreKeywordMid   = 50
	scoreSubsequence  = 60
	penaltyAction     = 5 // a tool ranks above its own actions
	penaltyOtherLang  = 3 // a match in the language not shown ranks lower
	maxContainsOffset = 9
)

// Index finds tools and their quick actions by name, pinyin, pinyin
// initials, ID and keywords. Pinyin is worked out once, when the index is
// built, so a keystroke only compares strings.
type Index struct {
	entries []entry
}

// entry is one searchable title in one language: a tool, or one of its
// quick actions.
type entry struct {
	owner    tool.Tool
	action   *tool.QuickAction
	lang     string
	title    string // "防睡眠" or "防睡眠：保持 1 小时"
	lower    string
	syllable [][]string // every reading of each rune pinyin spells
	sylRune  []int      // the rune of the title each syllable spells
	keywords []string   // lowercase ID and keywords
}

func newIndex(tools []tool.Tool) *Index {
	idx := &Index{}
	for _, t := range tools {
		m := t.Manifest()
		kw := make([]string, 0, 1+len(m.Keywords))
		kw = append(kw, strings.ToLower(m.ID))
		for _, k := range m.Keywords {
			kw = append(kw, strings.ToLower(k))
		}
		for _, lang := range Languages() {
			msgs := i18n.In(lang)
			idx.entries = append(idx.entries, newEntry(t, nil, lang, m.Name(msgs), kw))
			for i := range m.Actions {
				a := &m.Actions[i]
				title := msgs.ToolActionTitle(m.Name(msgs), a.Title(msgs))
				idx.entries = append(idx.entries, newEntry(t, a, lang, title, kw))
			}
		}
	}
	return idx
}

func newEntry(t tool.Tool, a *tool.QuickAction, lang, title string, kw []string) entry {
	e := entry{owner: t, action: a, lang: lang, title: title, lower: strings.ToLower(title), keywords: kw}
	args := pinyin.NewArgs()
	args.Heteronym = true // 重 is zhong and chong: "plcmm" finds 批量重命名
	for i, r := range []rune(title) {
		switch {
		case unicode.Is(unicode.Han, r):
			if readings := pinyin.SinglePinyin(r, args); len(readings) > 0 {
				e.syllable = append(e.syllable, slices.Compact(readings))
				e.sylRune = append(e.sylRune, i)
			}
		case r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			e.syllable = append(e.syllable, []string{string(unicode.ToLower(r))})
			e.sylRune = append(e.sylRune, i)
		}
	}
	return e
}

// Result is one match: a tool, or a quick action of a tool. Match is the
// runes [Match[0], Match[1]) of the title in the language shown that the
// query matched, for bold highlighting; equal when none.
type Result struct {
	Tool   tool.Tool
	Action *tool.QuickAction
	Match  [2]int
	score  int
}

// Title is the result's title in a language: a tool's name, or
// "tool: action".
func (r Result) Title(msgs i18n.Messages) string {
	m := r.Tool.Manifest()
	if r.Action == nil {
		return m.Name(msgs)
	}
	return msgs.ToolActionTitle(m.Name(msgs), r.Action.Title(msgs))
}

// Search returns the best matches of a query, at most limit of them,
// highlighting in the language lang. An empty query matches nothing.
func (idx *Index) Search(query, lang string, limit int) []Result {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	compact := strings.ReplaceAll(q, " ", "")
	type key struct{ tool, action string } // IDs: any tool value may be unhashable
	best := map[key]Result{}
	for i := range idx.entries {
		e := &idx.entries[i]
		r, ok := e.match(q, compact)
		if !ok {
			continue
		}
		if e.lang != lang {
			r.score += penaltyOtherLang
			r.Match = [2]int{}
		}
		k := key{tool: e.owner.Manifest().ID}
		if e.action != nil {
			k.action = e.action.ID
		}
		if old, seen := best[k]; !seen || r.score < old.score {
			best[k] = r
		}
	}
	out := make([]Result, 0, len(best))
	for _, r := range best {
		out = append(out, r)
	}
	slices.SortStableFunc(out, compareResults)
	return out[:min(limit, len(out))]
}

// compareResults orders by score, then tools before actions, then by ID,
// so equal scores keep a stable order.
func compareResults(a, b Result) int {
	if a.score != b.score {
		return a.score - b.score
	}
	return strings.Compare(a.Tool.Manifest().ID, b.Tool.Manifest().ID)
}

func (e *entry) match(q, compact string) (Result, bool) {
	r := Result{Tool: e.owner, Action: e.action}
	penalty := 0
	if e.action != nil {
		penalty = penaltyAction
	}
	score, from, to, ok := e.score(q, compact)
	if !ok {
		return r, false
	}
	r.score, r.Match = score+penalty, [2]int{from, to}
	return r, true
}

// score tries the ways of matching, best first.
func (e *entry) score(q, compact string) (score, from, to int, ok bool) {
	if i := strings.Index(e.lower, q); i >= 0 {
		from = len([]rune(e.lower[:i]))
		to = from + len([]rune(q))
		if i == 0 {
			return scorePrefix, from, to, true
		}
		return scoreContains + min(from, maxContainsOffset), from, to, true
	}
	if compact == "" {
		return 0, 0, 0, false
	}
	if from, to, ok := e.initials(compact); ok {
		return e.atStart(from, scoreInitials, scoreInitialsMid), from, to, true
	}
	if from, to, ok := e.spelled(compact); ok {
		return e.atStart(from, scoreSpelled, scoreSpelledMid), from, to, true
	}
	if s, ok := e.keyword(q); ok {
		return s, 0, 0, true
	}
	if e.subsequence(compact) {
		return scoreSubsequence, 0, 0, true
	}
	return 0, 0, 0, false
}

func (e *entry) atStart(from, start, mid int) int {
	if len(e.sylRune) > 0 && from == e.sylRune[0] {
		return start
	}
	return mid
}

func (e *entry) keyword(q string) (int, bool) {
	for _, k := range e.keywords {
		if strings.HasPrefix(k, q) {
			return scoreKeyword, true
		}
	}
	for _, k := range e.keywords {
		if strings.Contains(k, q) {
			return scoreKeywordMid, true
		}
	}
	return 0, false
}

// initials finds q as the first letters of consecutive syllables, any
// reading of each: "fsm" in 防睡眠.
func (e *entry) initials(q string) (from, to int, ok bool) {
	for start := 0; start+len(q) <= len(e.syllable); start++ {
		if e.initialsAt(start, q) {
			return e.sylRune[start], e.sylRune[start+len(q)-1] + 1, true
		}
	}
	return 0, 0, false
}

func (e *entry) initialsAt(start int, q string) bool {
	for k := range len(q) {
		if !hasInitial(e.syllable[start+k], q[k]) {
			return false
		}
	}
	return true
}

func hasInitial(readings []string, b byte) bool {
	for _, s := range readings {
		if s[0] == b {
			return true
		}
	}
	return false
}

// spelled finds q as the pinyin of consecutive syllables, the last one
// possibly typed in part: "fangshui", "fangshuim".
func (e *entry) spelled(q string) (from, to int, ok bool) {
	for start := range e.syllable {
		if last, ok := e.spell(start, q); ok {
			return e.sylRune[start], e.sylRune[last] + 1, true
		}
	}
	return 0, 0, false
}

func (e *entry) spell(i int, q string) (last int, ok bool) {
	if i >= len(e.syllable) {
		return 0, false
	}
	for _, s := range e.syllable[i] {
		if len(q) <= len(s) && strings.HasPrefix(s, q) {
			return i, true
		}
		if !strings.HasPrefix(q, s) {
			continue
		}
		if last, ok := e.spell(i+1, q[len(s):]); ok {
			return last, true
		}
	}
	return 0, false
}

// subsequence finds q's letters in order among the initials.
func (e *entry) subsequence(q string) bool {
	if len(q) < 2 {
		return false
	}
	j := 0
	for i := 0; i < len(e.syllable) && j < len(q); i++ {
		if hasInitial(e.syllable[i], q[j]) {
			j++
		}
	}
	return j == len(q)
}
