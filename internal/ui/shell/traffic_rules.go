package shell

import (
	"context"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// The views of the inspector.
const (
	viewFlows = iota
	viewRules
)

// The kinds of rule, in the order the New Rule menu lists them.
const (
	ruleRewrite = iota
	ruleMock
	ruleBlock
	ruleDelay
	ruleKindCount
)

// The methods a rule can match, in the order of their chips.
const (
	ruleMethodGET = iota
	ruleMethodPOST
	ruleMethodPUT
	ruleMethodDELETE
	ruleMethodPATCH
	ruleMethodHEAD
	ruleMethodOPTIONS
	ruleMethodCount
)

// Limits a rule's numbers must stay in, as the engine checks them: a
// status a server can answer, and a delay of at most five minutes.
const (
	ruleStatusMin = 200
	ruleStatusMax = 599
	ruleDelayMax  = 300000 // milliseconds
)

// ruleKind is the kind of the menu entry at i.
func ruleKind(i int) tool.RuleKind {
	switch i {
	case ruleRewrite:
		return tool.RuleRewrite
	case ruleMock:
		return tool.RuleMock
	case ruleBlock:
		return tool.RuleBlock
	}
	return tool.RuleDelay
}

// ruleKindLabel names a kind of rule.
func ruleKindLabel(k tool.RuleKind) string {
	switch k {
	case tool.RuleRewrite:
		return i18n.TrafficKindRewrite()
	case tool.RuleMock:
		return i18n.TrafficKindMock()
	case tool.RuleBlock:
		return i18n.TrafficKindBlock()
	case tool.RuleDelay:
		return i18n.TrafficKindDelay()
	}
	return ""
}

// ruleKindTone colours a kind of rule the way its effect reads: a rewrite
// accents, a mock answers, a block refuses and a delay waits.
func ruleKindTone(k tool.RuleKind) comp.Tone {
	switch k {
	case tool.RuleRewrite:
		return comp.ToneAccent
	case tool.RuleMock:
		return comp.ToneSuccess
	case tool.RuleBlock:
		return comp.ToneDanger
	case tool.RuleDelay:
		return comp.ToneWarning
	}
	return comp.ToneNeutral
}

// ruleMethod is the name of the method chip at i.
func ruleMethod(i int) string {
	switch i {
	case ruleMethodGET:
		return "GET"
	case ruleMethodPOST:
		return "POST"
	case ruleMethodPUT:
		return "PUT"
	case ruleMethodDELETE:
		return "DELETE"
	case ruleMethodPATCH:
		return "PATCH"
	case ruleMethodHEAD:
		return "HEAD"
	case ruleMethodOPTIONS:
		return "OPTIONS"
	}
	return ""
}

// ruleView is the state of the rule book and the form beside it.
type ruleView struct {
	loaded bool
	book   []tool.Rule // what the editor shows
	saved  []tool.Rule // what the tool last kept, to go back to on a failure
	sel    int
	list   ui.ListState
	dirty  bool
	due    time.Time
	err    error // the engine's refusal, in its own words
	form   ruleForm
	formID string // the rule the form's buffers belong to
	note   string // what a saved book says, shown once
}

// ruleName is what a rule is called, with a stand-in when it has no name.
func ruleName(r tool.Rule) string {
	if strings.TrimSpace(r.Name) == "" {
		return i18n.TrafficRuleUntitled()
	}
	return r.Name
}

// ruleLabel is the accessible name of a rule's row.
func ruleLabel(r tool.Rule) string {
	return ruleKindLabel(r.Kind) + " " + ruleName(r) + " " + r.Match.Pattern
}

// loadRules reads the book from the tool, once, and shows its first rule.
func (a *App) loadRules(t tool.Traffic, st *trafficState) {
	if st.rules.loaded {
		return
	}
	st.rules.loaded = true
	a.reloadRules(t, st)
}

// reloadRules reads the book from the tool again.
func (a *App) reloadRules(t tool.Traffic, st *trafficState) {
	st.rules.book = cloneRules(t.Rules())
	st.rules.saved = cloneRules(st.rules.book)
	st.rules.sel = min(max(st.rules.sel, 0), len(st.rules.book)-1)
	if len(st.rules.book) == 0 {
		st.rules.sel = -1
	}
	st.rules.formID = ""
}

// cloneRules copies a rule book deep enough that editing it never changes
// the copy the tool kept.
func cloneRules(in []tool.Rule) []tool.Rule {
	out := make([]tool.Rule, len(in))
	for i, r := range in {
		out[i] = r
		out[i].Match.Methods = slices.Clone(r.Match.Methods)
		out[i].Rewrite.RequestHeaders = slices.Clone(r.Rewrite.RequestHeaders)
		out[i].Rewrite.ResponseHeaders = slices.Clone(r.Rewrite.ResponseHeaders)
		out[i].Mock.Headers = slices.Clone(r.Mock.Headers)
		out[i].Rewrite.RequestBody = cloneBodyEdit(r.Rewrite.RequestBody)
		out[i].Rewrite.ResponseBody = cloneBodyEdit(r.Rewrite.ResponseBody)
	}
	return out
}

// cloneBodyEdit copies a body edit, or nothing.
func cloneBodyEdit(b *tool.BodyEdit) *tool.BodyEdit {
	if b == nil {
		return nil
	}
	cp := *b
	return &cp
}

// rulesDone applies what the tool said of the rule book.
func (st *trafficState) rulesDone(o trafficOutcome) {
	if o.sent == nil {
		return
	}
	if o.err != nil {
		st.rules.book = cloneRules(st.rules.saved)
		st.rules.err = o.err
		st.rules.formID = ""
		return
	}
	st.rules.saved = cloneRules(o.sent)
	st.rules.err = nil
	st.rules.note = i18n.TrafficRuleFlash()
}

// rulesDue keeps the book once typing has rested, as the config fields do.
func (a *App) rulesDue(c *ui.Context, t tool.Traffic, st *trafficState) {
	if !st.rules.dirty {
		return
	}
	if wait := time.Until(st.rules.due); wait > 0 {
		c.After(wait)
		return
	}
	a.rulesSet(t, st)
}

// rulesSet keeps the book through the tool, off the UI thread: the engine
// checks every rule and the file is written.
func (a *App) rulesSet(t tool.Traffic, st *trafficState) {
	st.rules.dirty = false
	sent := cloneRules(st.rules.book)
	st.begin()
	a.opt.Runner.Go(context.Background(), t.Manifest().ID, trafficTimeout, func(context.Context) {
		err := t.SetRules(sent)
		a.tell(st, trafficOutcome{err: err, sent: sent})
	})
}

// rulesChanged asks for the book to be kept once typing has rested.
func (st *trafficState) rulesChanged() {
	if len(st.rules.book) == 0 || st.rules.sel < 0 {
		return
	}
	st.rules.form.apply(&st.rules.book[st.rules.sel])
	st.rules.form.check()
	if st.rules.form.problems.any() {
		return
	}
	st.rules.dirty = true
	st.rules.due = time.Now().Add(cfgDelay)
}

// ruleNew adds a rule of a kind, chosen with the one pattern every rule
// starts with, and shows it.
func (a *App) ruleNew(t tool.Traffic, st *trafficState, kind tool.RuleKind) {
	r := tool.Rule{ID: nextRuleID(st.rules.book), Name: ruleKindLabel(kind), Enabled: true, Kind: kind}
	r.Match.Pattern = "*"
	if kind == tool.RuleMock {
		r.Mock.Status = ruleStatusMin
	}
	if kind == tool.RuleDelay {
		r.Delay = 500 * time.Millisecond
	}
	st.rules.book = append(st.rules.book, r)
	st.rules.sel = len(st.rules.book) - 1
	st.rules.formID = ""
	st.rules.dirty, st.rules.due = true, time.Now()
	a.rulesSet(t, st)
}

// nextRuleID is a rule id no rule of the book uses.
func nextRuleID(book []tool.Rule) string {
	used := make(map[string]bool, len(book))
	for _, r := range book {
		used[r.ID] = true
	}
	for n := 1; ; n++ {
		if id := "rule-" + strconv.Itoa(n); !used[id] {
			return id
		}
	}
}

// ruleDelete removes a rule after the person says so, and keeps the book.
func (a *App) ruleDelete(t tool.Traffic, st *trafficState) {
	if st.rules.sel < 0 || st.rules.sel >= len(st.rules.book) {
		return
	}
	r := st.rules.book[st.rules.sel]
	a.askDialog(comp.DialogOpts{
		Title:       i18n.TrafficRuleDeleteTitle(ruleName(r)),
		Description: i18n.TrafficRuleDeleteBody(),
		Cancel:      i18n.Cancel(),
		Confirm:     i18n.TrafficRuleDeleteConfirm(),
		Destructive: true,
		OnConfirm:   func() { a.ruleDrop(t, st) },
	})
}

// ruleDrop takes the chosen rule out of the book.
func (a *App) ruleDrop(t tool.Traffic, st *trafficState) {
	i := st.rules.sel
	st.rules.book = slices.Delete(slices.Clone(st.rules.book), i, i+1)
	st.rules.sel = min(i, len(st.rules.book)-1)
	st.rules.formID = ""
	st.rules.dirty, st.rules.due = true, time.Now()
	a.rulesSet(t, st)
}

// ruleDuplicate copies the chosen rule under a new id, right after it.
func (a *App) ruleDuplicate(t tool.Traffic, st *trafficState) {
	i := st.rules.sel
	if i < 0 {
		return
	}
	copyOf := st.rules.book[i]
	copyOf.ID = nextRuleID(st.rules.book)
	copyOf.Name = ruleName(st.rules.book[i]) + " " + strconv.Itoa(len(st.rules.book)+1)
	book := slices.Clone(st.rules.book)
	book = slices.Insert(book, i+1, cloneRules([]tool.Rule{copyOf})[0])
	st.rules.book = book
	st.rules.sel = i + 1
	st.rules.formID = ""
	st.rules.dirty, st.rules.due = true, time.Now()
	a.rulesSet(t, st)
}

// ruleMove moves the chosen rule one place earlier or later: the order is
// what decides which rule wins.
func (a *App) ruleMove(t tool.Traffic, st *trafficState, delta int) {
	i, j := st.rules.sel, st.rules.sel+delta
	if i < 0 || j < 0 || j >= len(st.rules.book) {
		return
	}
	book := slices.Clone(st.rules.book)
	book[i], book[j] = book[j], book[i]
	st.rules.book = book
	st.rules.sel = j
	st.rules.formID = ""
	st.rules.dirty, st.rules.due = true, time.Now()
	a.rulesSet(t, st)
}

// rulesView is the rule book: the list with its actions, and the form of
// the chosen rule.
func (a *App) rulesView(c *ui.Context, t tool.Traffic, st *trafficState) {
	ui.Column(c).Grow(1).FillWidth().Gap(ds.Space8).Children(func() {
		a.rulesToolbar(c, t, st)
		a.rulesChips(c, st)
		comp.Split(c, &st.rulesSplit, func() { a.rulesList(c, st) }, func() { a.ruleFormPane(c, st) }).
			Grow(1).FillWidth()
	})
}

// rulesToolbar is the line above the book: what order means, and what acts
// on the chosen rule.
func (a *App) rulesToolbar(c *ui.Context, t tool.Traffic, st *trafficState) {
	p := ds.P(c)
	ui.Row(c).FillWidth().Gap(ds.Space8).AlignItems(ui.Center).Children(func() {
		comp.Text(c, i18n.TrafficRulesHelp(), ds.Caption, p.TextInactive).Grow(1).Shrink(1).MinWidth(0)
		a.rulesActions(c, t, st)
	})
}

// rulesActions are the buttons that change the book: a new rule of a
// kind, and what to do with the chosen one.
func (a *App) rulesActions(c *ui.Context, t tool.Traffic, st *trafficState) {
	chosen := st.rules.sel >= 0
	newRule := a.kit.Button(c.Key("rule-new"), i18n.TrafficNewRule(), comp.ButtonOpts{Kind: comp.Primary, Small: true})
	newRule.Menu(func(m *ui.Menu) {
		for i := range ruleKindCount {
			kind := ruleKind(i)
			if m.Item(ruleKindLabel(kind)).Chosen() {
				a.ruleNew(t, st, kind)
			}
		}
	})
	up := comp.ButtonOpts{Kind: comp.Secondary, Small: true, Disabled: !chosen || st.rules.sel == 0}
	if a.kit.Button(c.Key("rule-up"), i18n.TrafficRuleMoveUp(), up).Clicked() {
		a.ruleMove(t, st, -1)
	}
	down := comp.ButtonOpts{Kind: comp.Secondary, Small: true, Disabled: !chosen || st.rules.sel == len(st.rules.book)-1}
	if a.kit.Button(c.Key("rule-down"), i18n.TrafficRuleMoveDown(), down).Clicked() {
		a.ruleMove(t, st, 1)
	}
	dupe := comp.ButtonOpts{Kind: comp.Secondary, Small: true, Disabled: !chosen}
	if a.kit.Button(c.Key("rule-dupe"), i18n.TrafficRuleDuplicate(), dupe).Clicked() {
		a.ruleDuplicate(t, st)
	}
	del := comp.ButtonOpts{Kind: comp.Danger, Small: true, Disabled: !chosen}
	if a.kit.Button(c.Key("rule-delete"), i18n.TrafficRuleDelete(), del).Clicked() {
		a.ruleDelete(t, st)
	}
}

// rulesChips says that the book was kept, once.
func (a *App) rulesChips(c *ui.Context, st *trafficState) {
	if st.rules.note == "" {
		return
	}
	c.Toast(st.rules.note)
	st.rules.note = ""
}

// rulesList is the book in order: the first rule at the top is the first
// one that matches.
func (a *App) rulesList(c *ui.Context, st *trafficState) {
	if len(st.rules.book) == 0 {
		comp.Card(c).FillHeight().FillWidth().Children(func() {
			ui.Column(c).Grow(1).FillWidth().Justify(ui.Center).AlignItems(ui.Center).Padding(ds.Space20).
				Children(func() {
					comp.Text(c, i18n.TrafficRulesEmpty(), ds.Caption, ds.P(c).TextInactive)
				})
		})
		return
	}
	st.rules.list.Selected = &st.rules.sel
	st.rules.list.Label = func(i int) string { return ruleLabel(st.rules.book[i]) }
	ui.List(c.Key("rules"), &st.rules.list, len(st.rules.book), func(i int) {
		a.ruleRow(c, st, i)
	}).Grow(1).FillWidth()
}

// ruleRow is one rule: whether it runs, its name, its kind, what it
// matches, and the methods when it takes a few.
func (a *App) ruleRow(c *ui.Context, st *trafficState, i int) {
	p := ds.P(c)
	r := st.rules.book[i]
	chosen := i == st.rules.sel
	b := ui.ButtonBase(c.Key(r.ID)).Label(ruleLabel(r)).Cursor(ui.CursorDefault)
	if b.Clicked() {
		st.rules.sel = i
	}
	b.FillWidth().Height(ds.SettingsRow).Padding(ds.Space6, ds.TableCellPad).Gap(ds.Space8).AlignItems(ui.Center)
	switch {
	case chosen:
		// An opaque fill, because the list marks the chosen row in the
		// accent colour behind this one; the bar says which row it is.
		b.Background(p.FillControl).BorderWidth(0, 0, 0, ds.FocusRingWidth).BorderColor(p.AccentStrong)
	case b.Hovered():
		b.Background(p.FillControl)
	}
	b.Children(func() {
		if comp.SwitchCompact(c.Key("rule-on"), &st.rules.book[i].Enabled, ruleName(r)).Changed() {
			st.rules.dirty, st.rules.due = true, time.Now()
		}
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(ds.Space2).Children(func() {
			ui.Row(c).FillWidth().Gap(ds.Space6).AlignItems(ui.Center).Children(func() {
				comp.Text(c, ruleName(r), ds.Label, p.TextStrong).Shrink(1).Ellipsis("…")
				comp.Badge(c, ruleKindLabel(r.Kind), ruleKindTone(r.Kind))
			})
			ui.Row(c).FillWidth().Gap(ds.Space4).AlignItems(ui.Center).Children(func() {
				comp.Value(c, r.Match.Pattern, p.TextSecondary).Grow(1).Shrink(1).MinWidth(0).Ellipsis("…")
				for _, m := range r.Match.Methods {
					comp.Badge(c, m, comp.ToneNeutral)
				}
			})
		})
	})
}

// ruleFormPane is the form of the chosen rule.
func (a *App) ruleFormPane(c *ui.Context, st *trafficState) {
	comp.Card(c).FillHeight().FillWidth().Clip().Children(func() {
		if st.rules.sel < 0 || st.rules.sel >= len(st.rules.book) {
			comp.Text(c, i18n.TrafficRulesEmpty(), ds.Caption, ds.P(c).TextInactive).Padding(ds.Space12)
			return
		}
		st.syncForm()
		ui.Scroll(c).Grow(1).FillWidth().Children(func() {
			ui.Column(c).FillWidth().Padding(ds.Space12).Gap(ds.Space10).Children(func() {
				a.ruleFormTitle(c, st)
				if st.rules.err != nil {
					a.ruleFormError(c, st)
				}
				a.ruleFormBody(c, st)
			})
		})
	})
}

// syncForm loads the form's buffers when another rule is chosen.
func (st *trafficState) syncForm() {
	r := st.rules.book[st.rules.sel]
	if st.rules.formID == r.ID {
		return
	}
	st.rules.formID = r.ID
	st.rules.form = loadRuleForm(r)
}

// ruleFormTitle is the rule's kind and its place in the order.
func (a *App) ruleFormTitle(c *ui.Context, st *trafficState) {
	p := ds.P(c)
	r := st.rules.book[st.rules.sel]
	ui.Row(c).FillWidth().Gap(ds.Space8).AlignItems(ui.Center).Children(func() {
		comp.Badge(c, ruleKindLabel(r.Kind), ruleKindTone(r.Kind))
		comp.Text(c, ruleName(r), ds.Headline, p.TextStrong).SingleLine().Ellipsis("…").Shrink(1)
		ui.Spacer(c)
		comp.Value(c, "#"+strconv.Itoa(st.rules.sel+1), p.TextMuted)
	})
}

// ruleFormError is the engine's refusal, under a headline that says what
// it is; the engine's own words are the detail.
func (a *App) ruleFormError(c *ui.Context, st *trafficState) {
	p := ds.P(c)
	ui.Column(c).FillWidth().Gap(ds.Space2).Padding(ds.Space8).Radius(ds.RadiusSm).
		Background(p.DangerSoft).Children(func() {
		comp.Text(c, i18n.TrafficRuleNotSaved(), ds.Label, p.Danger)
		comp.Text(c, st.rules.err.Error(), ds.Caption, p.TextPrimary).Selectable()
	})
}

// flowRules are the rules a chosen flow can become: a menu of the four
// things a person reaches for while looking at traffic.
func (a *App) flowRuleMenu(m *ui.Menu, t tool.Traffic, st *trafficState, f tool.FlowRow) {
	if m.Item(i18n.TrafficMockThis()).Chosen() {
		a.ruleFromFlow(t, st, f.ID, ruleForMock)
	}
	if m.Item(i18n.TrafficBlockThisURL()).Chosen() {
		a.ruleFromFlow(t, st, f.ID, ruleForBlockURL)
	}
	if m.Item(i18n.TrafficBlockThisHost()).Chosen() {
		a.ruleFromFlow(t, st, f.ID, ruleForBlockHost)
	}
	if m.Item(i18n.TrafficRuleRewriteResponse()).Chosen() {
		a.ruleFromFlow(t, st, f.ID, ruleForRewrite)
	}
}

// ruleFromFlow adds a rule built from a flow and shows it in the editor.
func (a *App) ruleFromFlow(t tool.Traffic, st *trafficState, id uint64, build func(tool.Flow) tool.Rule) {
	if st.log == nil {
		return
	}
	f, ok := st.log.Get(id)
	if !ok {
		return
	}
	r := build(f)
	r.ID = nextRuleID(st.rules.book)
	st.rules.book = append(st.rules.book, r)
	st.rules.sel = len(st.rules.book) - 1
	st.rules.formID = ""
	st.view = viewRules
	st.rules.dirty, st.rules.due = true, time.Now()
	a.rulesSet(t, st)
}

// ruleForMock is the rule that answers a flow's request as it answered:
// its status, its headers without the ones only the connection needs, and
// its body when that is text the capture kept whole.
func ruleForMock(f tool.Flow) tool.Rule {
	status := f.Status
	if status < ruleStatusMin {
		status = ruleStatusMin
	}
	return tool.Rule{
		Enabled: true,
		Kind:    tool.RuleMock,
		Match:   tool.RuleMatch{Pattern: f.URL, Methods: []string{f.Method}},
		Mock: tool.MockEffect{
			Status:  status,
			Headers: mockHeaders(f.Response.Headers),
			Body:    textBody(f.Response.Body),
		},
	}
}

// ruleForBlockURL is the rule that closes a flow's URL.
func ruleForBlockURL(f tool.Flow) tool.Rule {
	return tool.Rule{
		Enabled: true,
		Kind:    tool.RuleBlock,
		Match:   tool.RuleMatch{Pattern: f.URL, Methods: []string{f.Method}},
	}
}

// ruleForBlockHost is the rule that closes every request to a flow's host.
func ruleForBlockHost(f tool.Flow) tool.Rule {
	return tool.Rule{
		Enabled: true,
		Kind:    tool.RuleBlock,
		Match:   tool.RuleMatch{Pattern: "*://" + f.Host + "/*"},
	}
}

// ruleForRewrite is the rule that sends a flow's request to another URL.
func ruleForRewrite(f tool.Flow) tool.Rule {
	return tool.Rule{
		Enabled: true,
		Kind:    tool.RuleRewrite,
		Match:   tool.RuleMatch{Pattern: f.URL, Methods: []string{f.Method}},
	}
}

// mockHeaders drops what a connection or the capture itself decided, so
// that a mocked answer is one a server could have sent.
func mockHeaders(in []tool.Header) []tool.Header {
	out := make([]tool.Header, 0, len(in))
	for _, h := range in {
		switch strings.ToLower(strings.TrimSpace(h.Name)) {
		case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization",
			"te", "trailer", "transfer-encoding", "upgrade",
			"content-length", "content-encoding":
			continue
		}
		out = append(out, h)
	}
	return out
}

// textBody is a body as text, when it is text the capture kept whole; an
// image or anything else is left out of a mock.
func textBody(b tool.Body) string {
	if b.Truncated || len(b.Data) == 0 {
		return ""
	}
	_, kind := comp.FormatBody(b)
	switch kind {
	case comp.BodyJSON, comp.BodyText, comp.BodyForm:
		return string(b.Data)
	case comp.BodyEmpty, comp.BodyImage, comp.BodyBinary:
	}
	return ""
}

// ruleIndexByName finds a rule the book knows, for a hit a flow names.
func ruleIndexByName(book []tool.Rule, name string) int {
	for i, r := range book {
		if r.Name == name {
			return i
		}
	}
	return -1
}

// ruleFits reports whether a pattern is a sound rule: filled in, and a
// regular expression that compiles when it is read as one.
func ruleFits(pattern string, asRegexp bool) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return false
	}
	if !asRegexp {
		return true
	}
	_, err := regexp.Compile(pattern)
	return err == nil
}

// ruleStatus reads a status field: 0 when it is empty (the response keeps
// its own), and ok false when it is neither empty nor 200 to 599.
func ruleStatus(text string) (status int, ok bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, true
	}
	n, err := strconv.Atoi(text)
	if err != nil || n < ruleStatusMin || n > ruleStatusMax {
		return 0, false
	}
	return n, true
}

// ruleDelayMillis reads a delay field in milliseconds.
func ruleDelayMillis(text string) (d time.Duration, ok bool) {
	n, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || n < 0 || n > ruleDelayMax {
		return 0, false
	}
	return time.Duration(n) * time.Millisecond, true
}

// trafficTabs chooses between the captured traffic and the rule book.
func (a *App) trafficTabs(c *ui.Context, st *trafficState) {
	comp.Tabs(c.Key("traffic-view"), &st.view, i18n.TrafficView(), i18n.TrafficViewFlows(), i18n.TrafficViewRules())
}

// trafficRuleHits are the rules a flow names, each a way into its editor
// when the book knows it.
func (a *App) trafficRuleHits(c *ui.Context, st *trafficState, hits []string) {
	if len(hits) == 0 {
		return
	}
	p := ds.P(c)
	ui.Row(c).FillWidth().Gap(ds.Space8).AlignItems(ui.Start).Children(func() {
		comp.Text(c, i18n.TrafficRuleHits(), ds.Caption, p.TextInactive).Width(ds.SelectMinWidth).Shrink(0)
		ui.Row(c).Grow(1).Gap(ds.Space4).Wrap().AlignItems(ui.Center).Children(func() {
			for _, name := range hits {
				a.ruleHit(c, st, name)
			}
		})
	})
}

// ruleHit is one rule that fired: its name, and the way to edit it.
func (a *App) ruleHit(c *ui.Context, st *trafficState, name string) {
	index := ruleIndexByName(st.rules.book, name)
	b := a.kit.Button(c.Key("hit:"+name), name, comp.ButtonOpts{Kind: comp.Plain, Small: true})
	if index >= 0 {
		b.Tooltip(i18n.TrafficEditRule())
		if b.Clicked() {
			st.rules.sel, st.rules.formID = index, ""
			st.view = viewRules
		}
		return
	}
	comp.Value(c, name, ds.P(c).TextMuted)
}
