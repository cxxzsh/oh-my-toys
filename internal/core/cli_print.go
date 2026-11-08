package core

import (
	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

type planDoc struct {
	PlanID      string      `json:"plan_id"`
	Summary     string      `json:"summary"`
	Changes     []changeDoc `json:"changes"`
	ApplyLabel  string      `json:"apply_label"`
	Destructive bool        `json:"destructive"`
}

type changeDoc struct {
	Title  string `json:"title"`
	Detail string `json:"detail,omitempty"`
	Value  string `json:"value,omitempty"`
	Tone   string `json:"tone"`
}

type resultDoc struct {
	Summary string `json:"summary"`
}

type statusDoc struct {
	On     bool   `json:"on"`
	Text   string `json:"text"`
	Detail string `json:"detail,omitempty"`
	Note   string `json:"note,omitempty"`
}

func (c CLI) printPlan(id string, p tool.Plan) {
	if c.JSON {
		writeJSON(c.Out, planDocument(id, p))
		return
	}
	writef(c.Out, "%s\n", p.Summary)
	for _, r := range p.Rows {
		writef(c.Out, "%s\t%s\n", r.Title, r.Detail)
	}
	writef(c.Out, "%s\n", id)
}

func (c CLI) printResult(r tool.Result) {
	if c.JSON {
		writeJSON(c.Out, resultDoc{Summary: r.Summary})
		return
	}
	writef(c.Out, "%s\n", r.Summary)
}

func (c CLI) printStatus(s tool.Status) {
	if c.JSON {
		writeJSON(c.Out, statusDoc{On: s.On, Text: s.Text, Detail: s.Detail, Note: s.Note})
		return
	}
	writef(c.Out, "%s\n", s.Text)
	for _, line := range []string{s.Detail, s.Note} {
		if line != "" {
			writef(c.Out, "%s\n", line)
		}
	}
}

func planDocument(id string, p tool.Plan) planDoc {
	changes := make([]changeDoc, len(p.Rows))
	for i, r := range p.Rows {
		changes[i] = changeDoc{Title: r.Title, Detail: r.Detail, Value: r.Value, Tone: tool.ToneName(r.Tone)}
	}
	return planDoc{
		PlanID: id, Summary: p.Summary, Changes: changes,
		ApplyLabel: p.ApplyLabel, Destructive: p.Destructive,
	}
}
