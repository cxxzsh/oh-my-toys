package core

import (
	"testing"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

type fake struct{ m tool.Manifest }

func (f fake) Manifest() tool.Manifest { return f.m }

func (f fake) Specs() []tool.Spec { return nil }

func named(en, zh string) tool.Message {
	return func(m i18n.Messages) string {
		if m == i18n.In("zh-Hans") {
			return zh
		}
		return en
	}
}

func TestSearch(t *testing.T) {
	reg, err := NewRegistry(
		fake{tool.Manifest{
			ID: "awake", Name: named("Keep Awake", "防睡眠"), Keywords: []string{"caffeinate"},
			Actions: []tool.QuickAction{{ID: "1h", Title: named("1 hour", "保持 1 小时")}},
		}},
		fake{tool.Manifest{ID: "rename", Name: named("Batch Rename", "批量重命名")}},
		fake{tool.Manifest{ID: "json", Name: named("JSON Formatter", "JSON 格式化")}},
	)
	if err != nil {
		t.Fatal(err)
	}
	for q, want := range map[string]string{
		"fsm": "awake", "fangshui": "awake", "防睡": "awake", "keep": "awake", "caffe": "awake",
		"plcmm": "rename", "batch": "rename", "jsongsh": "json",
	} {
		rs := reg.Index().Search(q, "zh-Hans", 8)
		if len(rs) == 0 || rs[0].Tool.Manifest().ID != want {
			t.Errorf("%q: %v", q, rs)
		}
	}
	rs := reg.Index().Search("防睡", "zh-Hans", 8)
	if rs[0].Match != [2]int{0, 2} {
		t.Errorf("highlight %v", rs[0].Match)
	}
	if got := rs[len(rs)-1].Title(i18n.In("zh-Hans")); got != "防睡眠：保持 1 小时" {
		t.Errorf("action title %q", got)
	}
}
