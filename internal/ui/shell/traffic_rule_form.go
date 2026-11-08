package shell

import (
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// headerRow is one header edit or one mocked header, as the form shows it.
type headerRow struct {
	name, value string
	remove      bool
}

// bodyForm is one body edit: what to look for, what to write, and whether
// the find text is a regular expression.
type bodyForm struct {
	find, replace string
	regexp        bool
}

// ruleForm is the chosen rule while it is edited: text for every field,
// so that half-typed numbers and patterns stay as they were typed.
type ruleForm struct {
	kind                          tool.RuleKind
	name, pattern, url, status    string
	delay, file, body             string
	regexp                        bool
	reqBody, respBody             bodyForm
	methods                       [ruleMethodCount]bool
	reqHeaders, respHeaders, mock []headerRow
	problems                      ruleProblems
}

// ruleProblems are the fields that cannot be used yet, checked here so
// that the engine's refusal is not the first a person hears of it.
type ruleProblems struct {
	pattern, regexp, status, delay, header bool
}

// any reports whether anything is wrong.
func (p ruleProblems) any() bool {
	return p.pattern || p.regexp || p.status || p.delay || p.header
}

// loadRuleForm reads a rule into the form's buffers.
func loadRuleForm(r tool.Rule) ruleForm {
	f := ruleForm{
		kind:    r.Kind,
		name:    r.Name,
		pattern: r.Match.Pattern,
		regexp:  r.Match.Regexp,
		delay:   strconv.Itoa(int(r.Delay / time.Millisecond)),
	}
	for i := range f.methods {
		f.methods[i] = containsFold(r.Match.Methods, ruleMethod(i))
	}
	switch r.Kind {
	case tool.RuleRewrite:
		f.url = r.Rewrite.URL
		if r.Rewrite.Status > 0 {
			f.status = strconv.Itoa(r.Rewrite.Status)
		}
		f.reqHeaders = headerRows(r.Rewrite.RequestHeaders)
		f.respHeaders = headerRows(r.Rewrite.ResponseHeaders)
		f.reqBody = loadBodyForm(r.Rewrite.RequestBody)
		f.respBody = loadBodyForm(r.Rewrite.ResponseBody)
	case tool.RuleMock:
		f.status = strconv.Itoa(r.Mock.Status)
		f.mock = mockHeaderRows(r.Mock.Headers)
		f.body = r.Mock.Body
		f.file = r.Mock.File
	case tool.RuleBlock, tool.RuleDelay:
	}
	return f
}

// headerRows reads header edits into form rows.
func headerRows(in []tool.HeaderEdit) []headerRow {
	out := make([]headerRow, len(in))
	for i, h := range in {
		out[i] = headerRow{name: h.Name, value: h.Value, remove: h.Remove}
	}
	return out
}

// mockHeaderRows reads mocked headers into form rows.
func mockHeaderRows(in []tool.Header) []headerRow {
	out := make([]headerRow, len(in))
	for i, h := range in {
		out[i] = headerRow{name: h.Name, value: h.Value}
	}
	return out
}

// loadBodyForm reads a body edit into the form, or nothing.
func loadBodyForm(b *tool.BodyEdit) bodyForm {
	if b == nil {
		return bodyForm{}
	}
	return bodyForm{find: b.Find, replace: b.Replace, regexp: b.Regexp}
}

// headerEdits reads the form's rows as header edits.
func headerEdits(rows []headerRow) []tool.HeaderEdit {
	var out []tool.HeaderEdit
	for _, r := range rows {
		if strings.TrimSpace(r.name) == "" {
			continue
		}
		out = append(out, tool.HeaderEdit{Name: strings.TrimSpace(r.name), Value: r.value, Remove: r.remove})
	}
	return out
}

// bodyEditOf reads a body form as a body edit: nothing when there is
// nothing to do, which leaves the body as it is.
func bodyEditOf(f bodyForm) *tool.BodyEdit {
	if f.find == "" && f.replace == "" {
		return nil
	}
	return &tool.BodyEdit{Find: f.find, Replace: f.replace, Regexp: f.regexp}
}

// apply writes the form's buffers into a rule, keeping the values of
// fields that cannot be used as they were.
func (f *ruleForm) apply(r *tool.Rule) {
	r.Name = strings.TrimSpace(f.name)
	r.Match.Pattern = strings.TrimSpace(f.pattern)
	r.Match.Regexp = f.regexp
	r.Match.Methods = nil
	for i := range f.methods {
		if f.methods[i] {
			r.Match.Methods = append(r.Match.Methods, ruleMethod(i))
		}
	}
	switch r.Kind {
	case tool.RuleRewrite:
		r.Rewrite.URL = strings.TrimSpace(f.url)
		if status, ok := ruleStatus(f.status); ok {
			r.Rewrite.Status = status
		}
		r.Rewrite.RequestHeaders = headerEdits(f.reqHeaders)
		r.Rewrite.ResponseHeaders = headerEdits(f.respHeaders)
		r.Rewrite.RequestBody = bodyEditOf(f.reqBody)
		r.Rewrite.ResponseBody = bodyEditOf(f.respBody)
	case tool.RuleMock:
		if status, ok := ruleStatus(f.status); ok && status >= ruleStatusMin {
			r.Mock.Status = status
		}
		r.Mock.Headers = mockHeadersOf(f.mock)
		r.Mock.Body = f.body
		r.Mock.File = strings.TrimSpace(f.file)
	case tool.RuleDelay:
		if d, ok := ruleDelayMillis(f.delay); ok {
			r.Delay = d
		}
	case tool.RuleBlock:
	}
}

// mockHeadersOf reads the form's rows as mocked headers.
func mockHeadersOf(rows []headerRow) []tool.Header {
	var out []tool.Header
	for _, r := range rows {
		if strings.TrimSpace(r.name) == "" {
			continue
		}
		out = append(out, tool.Header{Name: strings.TrimSpace(r.name), Value: r.value})
	}
	return out
}

// check finds what cannot be used yet: an empty pattern, one that does not
// compile, a status or a delay out of range, a header row without a name.
func (f *ruleForm) check() {
	f.problems = ruleProblems{}
	f.problems.pattern = !ruleFits(f.pattern, false)
	f.problems.regexp = f.regexp && strings.TrimSpace(f.pattern) != "" && !ruleFits(f.pattern, true)
	_, ok := ruleStatus(f.status)
	f.problems.status = !ok || (f.kind == tool.RuleMock && strings.TrimSpace(f.status) == "")
	if f.kind == tool.RuleDelay {
		_, ok := ruleDelayMillis(f.delay)
		f.problems.delay = !ok
	}
	for _, rows := range [][]headerRow{f.reqHeaders, f.respHeaders, f.mock} {
		for _, r := range rows {
			if strings.TrimSpace(r.name) == "" && (r.value != "" || r.remove) {
				f.problems.header = true
			}
		}
	}
}

// containsFold reports whether a method list holds a name.
func containsFold(list []string, name string) bool {
	for _, s := range list {
		if strings.EqualFold(s, name) {
			return true
		}
	}
	return false
}

// ruleFormBody is every field of the chosen rule, the ones its kind has.
func (a *App) ruleFormBody(c *ui.Context, st *trafficState) {
	f := &st.rules.form
	f.check()
	p := ds.P(c)
	a.ruleNameField(c, st)
	a.ruleMatchFields(c, st)
	switch f.kind {
	case tool.RuleRewrite:
		a.ruleRewriteFields(c, st)
	case tool.RuleMock:
		a.ruleMockFields(c, st)
	case tool.RuleBlock:
		comp.Text(c, i18n.TrafficRuleBlockNote(), ds.Caption, p.TextInactive)
	case tool.RuleDelay:
		o := comp.FieldOpts{Label: i18n.TrafficRuleDelay(), Help: i18n.TrafficRuleDelayHint(), Mono: true}
		if f.problems.delay {
			o.Help, o.Error = "", i18n.TrafficRuleDelayBad()
		}
		if comp.TextField(c.Key("rule-delay"), &f.delay, o).Changed() {
			st.rulesChanged()
		}
	}
}

// ruleNameField is what the rule is called.
func (a *App) ruleNameField(c *ui.Context, st *trafficState) {
	f := &st.rules.form
	if comp.TextField(c.Key("rule-name"), &f.name, comp.FieldOpts{Label: i18n.TrafficRuleName()}).Changed() {
		st.rulesChanged()
	}
}

// ruleMatchFields are the pattern, how to read it, and the methods.
func (a *App) ruleMatchFields(c *ui.Context, st *trafficState) {
	f := &st.rules.form
	p := ds.P(c)
	o := comp.FieldOpts{Label: i18n.TrafficRulePattern(), Placeholder: i18n.TrafficRulePatternHint(), Mono: true}
	if f.problems.pattern {
		o.Error = i18n.TrafficRulePatternBad()
	} else if f.problems.regexp {
		o.Error = i18n.TrafficRuleRegexpBad()
	}
	if comp.TextField(c.Key("rule-pattern"), &f.pattern, o).Changed() {
		st.rulesChanged()
	}
	ui.Row(c).Gap(ds.Space8).AlignItems(ui.Center).Children(func() {
		if comp.Switch(c.Key("rule-regexp"), &f.regexp, i18n.TrafficRuleRegexp()).Changed() {
			st.rulesChanged()
		}
		comp.Text(c, i18n.TrafficRuleRegexp(), ds.Label, p.TextPrimary)
	})
	ui.Row(c).FillWidth().Gap(ds.Space6).AlignItems(ui.Center).Wrap().Children(func() {
		comp.Eyebrow(c, i18n.TrafficRuleMethods())
		for i := range f.methods {
			if comp.Chip(c.Key("rule-method:"+ruleMethod(i)), ruleMethod(i), f.methods[i]).Clicked() {
				f.methods[i] = !f.methods[i]
				st.rulesChanged()
			}
		}
	})
}

// ruleRewriteFields are a rewrite's redirect, status, header edits and
// body edits.
func (a *App) ruleRewriteFields(c *ui.Context, st *trafficState) {
	f := &st.rules.form
	o := comp.FieldOpts{Label: i18n.TrafficRuleRedirect(), Placeholder: i18n.TrafficRuleRedirectHint(), Mono: true}
	if comp.TextField(c.Key("rule-url"), &f.url, o).Changed() {
		st.rulesChanged()
	}
	a.ruleStatusField(c, st)
	a.headerList(c, st, "req-head", i18n.TrafficRuleReqHeaders(), &f.reqHeaders)
	a.headerList(c, st, "resp-head", i18n.TrafficRuleRespHeaders(), &f.respHeaders)
	a.bodyEditFields(c, st, "req-body", i18n.TrafficRuleReqBody(), &f.reqBody)
	a.bodyEditFields(c, st, "resp-body", i18n.TrafficRuleRespBody(), &f.respBody)
}

// ruleMockFields are a mock's status, headers, body and local file.
func (a *App) ruleMockFields(c *ui.Context, st *trafficState) {
	f := &st.rules.form
	a.ruleStatusField(c, st)
	a.headerList(c, st, "mock-head", i18n.TrafficHeaders(), &f.mock)
	a.mockBodyField(c, st)
	a.mockFileField(c, st)
}

// ruleStatusField is the status a rule answers or rewrites to.
func (a *App) ruleStatusField(c *ui.Context, st *trafficState) {
	f := &st.rules.form
	o := comp.FieldOpts{Label: i18n.StatusLabel(), Placeholder: i18n.TrafficRuleStatusKeep(), Mono: true}
	if f.problems.status {
		o.Error = i18n.TrafficRuleStatusBad()
	}
	if comp.TextField(c.Key("rule-status"), &f.status, o).Changed() {
		st.rulesChanged()
	}
}

// headerList is one list of header edits, with the ways to change it.
func (a *App) headerList(c *ui.Context, st *trafficState, key, title string, rows *[]headerRow) {
	comp.Eyebrow(c, title)
	for i := range *rows {
		a.headerRow(c, st, key, rows, i)
	}
	if a.kit.Button(c.Key(key+"-add"), i18n.TrafficRuleAddHeader(), comp.ButtonOpts{Kind: comp.Ghost, Small: true}).Clicked() {
		*rows = append(*rows, headerRow{})
		st.rulesChanged()
	}
}

// headerRow is one header edit: its name, its value, whether it goes, and
// the way to take the row away.
func (a *App) headerRow(c *ui.Context, st *trafficState, key string, rows *[]headerRow, i int) {
	r := &(*rows)[i]
	o := comp.FieldOpts{Placeholder: i18n.TrafficRuleHeaderName(), Mono: true}
	if r.name == "" && (r.value != "" || r.remove) {
		o.Error = i18n.TrafficRuleHeaderBad()
	}
	ui.Row(c.Key(i)).FillWidth().Gap(ds.Space6).AlignItems(ui.End).Children(func() {
		ui.Box(c).Grow(2).Basis(0).Children(func() {
			if comp.TextField(c, &r.name, o).Changed() {
				st.rulesChanged()
			}
		})
		ui.Box(c).Grow(3).Basis(0).Children(func() {
			v := comp.FieldOpts{Placeholder: i18n.TrafficRuleHeaderValue(), Mono: true}
			if comp.TextField(c, &r.value, v).Changed() {
				st.rulesChanged()
			}
		})
		if comp.SwitchCompact(c.Key("rm"), &r.remove, i18n.TrafficRuleHeaderRemove()).Changed() {
			st.rulesChanged()
		}
		if a.kit.Button(c.Key(key+"-del"), i18n.TrafficRuleDelete(), comp.ButtonOpts{Kind: comp.Plain, Small: true}).Clicked() {
			*rows = append((*rows)[:i], (*rows)[i+1:]...)
			st.rulesChanged()
		}
	})
}

// bodyEditFields is one body edit: what to find, what to write instead,
// and whether the find text is a regular expression.
func (a *App) bodyEditFields(c *ui.Context, st *trafficState, key, title string, b *bodyForm) {
	p := ds.P(c)
	comp.Eyebrow(c, title)
	ui.Row(c).FillWidth().Gap(ds.Space6).AlignItems(ui.End).Children(func() {
		ui.Box(c).Grow(1).Basis(0).Children(func() {
			find := comp.FieldOpts{Placeholder: i18n.TrafficRuleFind(), Mono: true}
			if comp.TextField(c, &b.find, find).Changed() {
				st.rulesChanged()
			}
		})
		ui.Box(c).Grow(1).Basis(0).Children(func() {
			repl := comp.FieldOpts{Placeholder: i18n.TrafficRuleReplace(), Mono: true}
			if comp.TextField(c, &b.replace, repl).Changed() {
				st.rulesChanged()
			}
		})
		if comp.SwitchCompact(c.Key(key+"-re"), &b.regexp, i18n.TrafficRuleRegexp()).Changed() {
			st.rulesChanged()
		}
	})
	comp.Text(c, i18n.TrafficRuleBodyHelp(), ds.Caption, p.TextInactive)
}

// mockBodyField is the body a mock answers with, in a multi-line field.
func (a *App) mockBodyField(c *ui.Context, st *trafficState) {
	f := &st.rules.form
	p := ds.P(c)
	comp.Text(c, i18n.TrafficBody(), ds.Label, p.TextPrimary)
	code := ds.Body
	code.Mono = true
	ta := ui.TextArea(c, &f.body).Label(i18n.TrafficBody())
	ds.Style(ta, code).FillWidth().Height(ds.ControlLg*4).Padding(ds.Space8).Radius(ds.RadiusSm).
		Background(p.FillControl).Border(ds.LineWidth, p.LineHairline).TextColor(p.TextPrimary)
	if ta.Changed() {
		st.rulesChanged()
	}
}

// mockFileField is the local file a mock serves instead of its body.
func (a *App) mockFileField(c *ui.Context, st *trafficState) {
	f := &st.rules.form
	p := ds.P(c)
	ui.Row(c).FillWidth().Gap(ds.Space8).AlignItems(ui.End).Children(func() {
		ui.Box(c).Grow(1).Basis(0).Children(func() {
			o := comp.FieldOpts{Label: i18n.TrafficRuleMockFile(), Mono: true}
			if comp.TextField(c.Key("rule-file"), &f.file, o).Changed() {
				st.rulesChanged()
			}
		})
		if a.kit.Button(c.Key("rule-file-choose"), i18n.TrafficRuleChooseFile(), comp.ButtonOpts{Kind: comp.Secondary, Small: true}).Clicked() {
			a.pickRuleFile(st)
		}
		if f.file != "" {
			if a.kit.Button(c.Key("rule-file-clear"), i18n.TrafficRuleClearFile(), comp.ButtonOpts{Kind: comp.Ghost, Small: true}).Clicked() {
				f.file = ""
				st.rulesChanged()
			}
		}
	})
	comp.Text(c, i18n.TrafficRuleMockFileHint(), ds.Caption, p.TextInactive)
}

// pickRuleFile asks the system for the file a mock serves.
func (a *App) pickRuleFile(st *trafficState) {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.main, Title: i18n.TrafficRuleMockFile()})
		if err != nil || len(paths) == 0 {
			return
		}
		a.main.Update(func() {
			st.rules.form.file = paths[0]
			st.rulesChanged()
			a.redraw()
		})
	}()
}
