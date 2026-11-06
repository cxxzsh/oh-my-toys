package devtools

import (
	"context"
	"errors"

	"github.com/cxxzsh/oh-my-toys/spikes/mygo/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/spikes/mygo/internal/tool"
)

// CrashID is the crash tool's ID.
const CrashID = "crash"

const fieldWhere = "where"

// Crash is a tool that fails on purpose: its plan panics when the field
// says "plan", and its quick action panics, so a test can check that the
// rest of the app goes on.
type Crash struct{}

// Manifest describes the tool.
func (Crash) Manifest() tool.Manifest {
	return tool.Manifest{
		ID:       CrashID,
		Name:     named("Crash Test", "崩溃测试"),
		Summary:  named("Fails on purpose: type “plan” to panic while planning", "故意出错：输入 plan 会在预览时 panic"),
		Category: tool.CategoryDev,
		Kind:     tool.KindAction,
		Icon:     "bug",
		Keywords: []string{"panic", "crash"},
		Actions: []tool.QuickAction{{
			ID: "panic", Title: named("Panic in an action", "在动作里 panic"),
			Run: func(context.Context) error { panic("crash test: a quick action panicked") },
		}},
	}
}

// Specs declares the crash test's one action, a plan.
func (Crash) Specs() []tool.Spec {
	return []tool.Spec{tool.PlanSpec(named("Fails on purpose", "故意出错"), tool.Schema{
		Type: tool.TypeObject, Properties: []tool.Property{{
			Name:   fieldWhere,
			Schema: tool.Schema{Type: tool.TypeString, Title: named("Where", "在哪里出错"), Hint: named("plan", "plan")},
		}},
	})}
}

// Plan panics when asked to.
func (Crash) Plan(_ context.Context, in tool.Input) (tool.Plan, error) {
	if in[fieldWhere] == "plan" {
		panic("crash test: Plan panicked")
	}
	return tool.Plan{Summary: i18n.StatusOn()}, nil
}

// Apply always fails.
func (Crash) Apply(context.Context, tool.Plan) (tool.Result, error) {
	return tool.Result{}, errors.New("crash test: Apply failed")
}
