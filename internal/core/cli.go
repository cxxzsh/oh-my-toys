package core

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// flagJSON is the global flag that asks for one JSON document. Its text is
// never translated: the caller sets the catalog to the source language.
const flagJSON = "--json"

// OutputLocale is the language a run shows. JSON stays in the source
// language, so its wording does not follow the system language.
func OutputLocale(jsonMode bool, system string) string {
	if jsonMode {
		return ""
	}
	return system
}

// CLI runs one omt invocation against a registry. It changes nothing unless
// the action is apply, start or stop, and apply of a write or process
// action asks first.
type CLI struct {
	Registry *Registry
	Plans    PlanStore
	Msgs     i18n.Messages
	In       io.Reader
	Out      io.Writer
	Err      io.Writer
	JSON     bool
	Terminal bool // a person can answer a prompt
}

// Run parses args, with the leading program name already removed, and
// returns an exit code.
func (c CLI) Run(ctx context.Context, args []string) int {
	if slices.Contains(args, flagJSON) {
		c.JSON = true
	}
	args = withoutJSON(args)
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		c.listTools()
		return ExitOK
	}
	t := c.Registry.Get(args[0])
	if t == nil {
		return c.usage(c.Msgs.CliUnknownTool(args[0]))
	}
	if len(args) == 1 || args[1] == "--help" || args[1] == "-h" {
		c.listActions(t)
		return ExitOK
	}
	spec, ok := SpecOf(t, args[1])
	if !ok {
		return c.usage(c.Msgs.CliUnknownAction(args[0], args[1]))
	}
	in, uerr := parseFlags(c.Msgs, args[2:], spec)
	if uerr != nil {
		return c.usage(uerr.Error())
	}
	return c.dispatch(ctx, t, spec, in)
}

func (c CLI) dispatch(ctx context.Context, t tool.Tool, spec tool.Spec, in tool.Input) int {
	switch spec.Name {
	case tool.ActionPlan:
		act, ok := t.(tool.Action)
		if !ok {
			return c.usage(c.Msgs.CliUnknownAction(t.Manifest().ID, spec.Name))
		}
		return c.plan(ctx, t.Manifest().ID, act, in)
	case tool.ActionApply:
		act, ok := t.(tool.Action)
		if !ok {
			return c.usage(c.Msgs.CliUnknownAction(t.Manifest().ID, spec.Name))
		}
		return c.apply(ctx, t.Manifest().ID, act, spec, in)
	case tool.ActionStart, tool.ActionStop, tool.ActionStatus:
		sw, ok := t.(tool.Switch)
		if !ok {
			return c.usage(c.Msgs.CliUnknownAction(t.Manifest().ID, spec.Name))
		}
		return c.switchRun(ctx, sw, spec.Name)
	default:
		return c.usage(c.Msgs.CliUnknownAction(t.Manifest().ID, spec.Name))
	}
}

func (c CLI) plan(ctx context.Context, id string, act tool.Action, in tool.Input) int {
	p, err := act.Plan(ctx, in)
	if err != nil {
		return c.fail(err)
	}
	planID, err := c.Plans.save(id, in, p)
	if err != nil {
		return c.fail(err)
	}
	c.printPlan(planID, p)
	if p.ApplyLabel != "" && !c.JSON {
		writef(c.Out, "%s\n", c.Msgs.CliApplyHint(id, planID))
	}
	return ExitOK
}

func (c CLI) apply(ctx context.Context, id string, act tool.Action, spec tool.Spec, in tool.Input) int {
	saved, err := c.Plans.load(in["plan_id"], id)
	if err != nil {
		return c.planFail(in["plan_id"], err)
	}
	fresh, err := act.Plan(ctx, saved.Inputs)
	if err != nil {
		return c.fail(err)
	}
	if !saved.Matches(fresh) {
		return c.failText(c.Msgs.CliPlanStale(saved.ID))
	}
	if code := c.confirm(spec, fresh.Summary); code != ExitOK {
		return code
	}
	res, err := act.Apply(ctx, fresh)
	if err != nil {
		return c.fail(err)
	}
	c.printResult(res)
	return ExitOK
}

func (c CLI) confirm(spec tool.Spec, summary string) int {
	if spec.Risk != tool.RiskWrite && spec.Risk != tool.RiskProcess {
		return ExitOK
	}
	if !c.Terminal {
		return c.confirmFail()
	}
	writef(c.Out, "%s", c.Msgs.CliConfirm(summary))
	line, err := readLine(c.In)
	if err != nil {
		return c.confirmFail()
	}
	if strings.EqualFold(line, "y") || strings.EqualFold(line, "yes") {
		return ExitOK
	}
	writef(c.Err, "%s\n", c.Msgs.CliRefused())
	return ExitDenied
}

func (c CLI) confirmFail() int {
	writef(c.Err, "%s\n", c.Msgs.CliConfirmRequired())
	return ExitConfirm
}

func (c CLI) switchRun(ctx context.Context, sw tool.Switch, action string) int {
	switch action {
	case tool.ActionStart:
		return c.hold(ctx, sw)
	case tool.ActionStop:
		if err := sw.Stop(); err != nil {
			return c.fail(err)
		}
	}
	c.printStatus(sw.Status())
	return ExitOK
}

// hold runs a switch tool until ctx ends, then stops it. Without the
// desktop app, the command itself is what keeps the tool running.
func (c CLI) hold(ctx context.Context, sw tool.Switch) int {
	if err := sw.Start(ctx); err != nil {
		return c.fail(err)
	}
	c.printStatus(sw.Status())
	if !c.JSON {
		writef(c.Err, "%s\n", c.Msgs.CliHolding())
	}
	<-ctx.Done()
	if err := sw.Stop(); err != nil {
		return c.fail(err)
	}
	return ExitOK
}

func (c CLI) planFail(id string, err error) int {
	var wrong *wrongToolError
	switch {
	case errors.As(err, &wrong):
		return c.failText(c.Msgs.CliPlanTool(id, wrong.Tool))
	case errors.Is(err, errPlanExpired):
		return c.failText(c.Msgs.CliPlanExpired(id))
	default:
		return c.failText(c.Msgs.CliPlanMissing(id))
	}
}

func (c CLI) fail(err error) int {
	var problem *tool.Problem
	if errors.As(err, &problem) && problem.Text != "" {
		return c.failText(problem.Text)
	}
	return c.failText(err.Error())
}

func (c CLI) failText(text string) int {
	writef(c.Err, "%s\n", text)
	return ExitError
}

func (c CLI) usage(text string) int {
	writef(c.Err, "%s\n", text)
	return ExitUsage
}

func withoutJSON(args []string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		if a != flagJSON {
			out = append(out, a)
		}
	}
	return out
}

func readLine(r io.Reader) (string, error) {
	line, err := bufio.NewReader(r).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func writeJSON(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

// writef writes to a terminal stream. A short write there is not a tool failure.
func writef(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}
