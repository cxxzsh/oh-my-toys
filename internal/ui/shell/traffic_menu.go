package shell

import (
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// trafficFlowMenu is what a person can do with one flow besides looking at
// it. Each action goes through the tool contract only.
func (a *App) trafficFlowMenu(m *ui.Menu, c *ui.Context, t tool.Traffic, st *trafficState, r tool.FlowRow) {
	if r.Source == tool.SourceProxy && m.Item(i18n.TrafficReplay()).Chosen() {
		a.trafficReplay(t, st, r.ID)
	}
	if m.Item(i18n.TrafficCopyURL()).Chosen() {
		a.trafficCopyFlow(c, st, r.ID, func(f tool.Flow) string { return f.URL })
	}
	if m.Item(i18n.TrafficMenuCopyCurl()).Chosen() {
		a.trafficCopyFlow(c, st, r.ID, curlCommand)
	}
	if m.Item(i18n.TrafficFilterHost()).Chosen() {
		st.addQueryToken(queryHost, r.Host)
	}
	if m.Item(i18n.TrafficMenuFilterProgram()).Chosen() {
		st.addQueryToken(queryProgram, r.Process.Name)
	}
	m.Separator()
	a.flowRuleMenu(m, t, st, r)
	if m.Item(i18n.TrafficMenuBlockHost()).Chosen() {
		a.trafficBlockHost(c, t, st, r)
	}
	if m.Item(i18n.TrafficMenuDecrypt()).Chosen() {
		a.trafficBypassHost(c, t, st, r.Host)
	}
	if m.Item(i18n.TrafficMenuCaptureProgram()).Disabled(r.Process.Name == "").Chosen() {
		a.trafficScopeProgram(c, t, st, r.Process.Name)
	}
}

// trafficFlowOf reads one flow, which the list rows do not carry in full.
func trafficFlowOf(st *trafficState, id uint64) (tool.Flow, bool) {
	if st.log == nil {
		return tool.Flow{}, false
	}
	return st.log.Get(id)
}

// trafficCopyFlow copies a part of one flow to the clipboard and says so.
func (a *App) trafficCopyFlow(c *ui.Context, st *trafficState, id uint64, part func(tool.Flow) string) {
	f, ok := trafficFlowOf(st, id)
	if !ok {
		return
	}
	text := part(f)
	mygo.Clipboard.WriteText(text)
	c.Toast(i18n.Copied(text))
}

// addQueryToken appends a key:value token to the search text. A value with
// spaces or quotes is quoted so the parser reads it back whole.
func (st *trafficState) addQueryToken(key, value string) {
	if value == "" {
		return
	}
	if strings.ContainsAny(value, " \t\"") {
		value = `"` + value + `"`
	}
	st.text = strings.TrimSpace(st.text + " " + key + ":" + value)
}

// trafficBlockHost adds the host's block rule, the one ruleForBlockHost
// builds, unless an identical enabled one is already in the book.
func (a *App) trafficBlockHost(c *ui.Context, t tool.Traffic, st *trafficState, r tool.FlowRow) {
	if t == nil || r.Host == "" {
		return
	}
	book := t.Rules()
	rule := ruleForBlockHost(tool.Flow{FlowRow: r})
	for _, have := range book {
		if have.Enabled && have.Kind == tool.RuleBlock && have.Match.Pattern == rule.Match.Pattern {
			c.Toast(i18n.TrafficMenuBlocked(r.Host))
			return
		}
	}
	rule.ID = nextRuleID(book)
	rule.Name = "Block " + r.Host
	if err := t.SetRules(append(book, rule)); err != nil {
		a.trafficMenuFail(c, st, err)
		return
	}
	c.Toast(i18n.TrafficMenuBlocked(r.Host))
}

// trafficBypassHost adds the host to the hosts that are never decrypted,
// once.
func (a *App) trafficBypassHost(c *ui.Context, t tool.Traffic, st *trafficState, host string) {
	if t == nil || host == "" {
		return
	}
	cfg := t.Config()
	if !slices.Contains(cfg.Bypass, host) {
		cfg.Bypass = append(slices.Clone(cfg.Bypass), host)
		if err := t.SetConfig(cfg); err != nil {
			a.trafficMenuFail(c, st, err)
			return
		}
	}
	c.Toast(i18n.TrafficMenuBypassed(host))
}

// trafficScopeProgram records only the traffic of one program.
func (a *App) trafficScopeProgram(c *ui.Context, t tool.Traffic, st *trafficState, name string) {
	if t == nil || name == "" {
		return
	}
	cfg := t.Config()
	cfg.Scope = []string{name}
	if err := t.SetConfig(cfg); err != nil {
		a.trafficMenuFail(c, st, err)
		return
	}
	c.Toast(i18n.TrafficMenuScoped(name))
}

// trafficMenuFail shows the failure on the page banner, in plain words.
func (a *App) trafficMenuFail(c *ui.Context, st *trafficState, err error) {
	st.err = err
	title, _ := trafficProblemText(err)
	c.Toast(title)
}
