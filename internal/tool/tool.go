// Package tool is the contract every tool implements: a manifest, and
// Start/Stop/Status for switch tools or Plan/Apply for action tools. It
// sits below the tools, which implement it, and the core, which runs them.
package tool

import (
	"context"
	"fmt"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
)

// Message is a catalog message in a given language, such as
// i18n.Messages.AwakeName. Manifests hold messages, never strings, so the
// same manifest reads in every language.
type Message = func(i18n.Messages) string

// Kind tells switch tools, which keep running, from action tools.
type Kind int

// The two kinds of tools.
const (
	KindSwitch Kind = iota
	KindAction
)

// Category groups tools; the UI colors tools by category, never by tool.
type Category string

// The categories, in the order the UI lists them.
const (
	CategorySystem       Category = "system"
	CategoryFile         Category = "file"
	CategoryDev          Category = "dev"
	CategoryNetwork      Category = "network"
	CategoryText         Category = "text"
	CategoryMedia        Category = "media"
	CategoryProductivity Category = "productivity"
	CategoryApp          Category = "app"
)

// Categories lists the categories tools may belong to, in display order.
func Categories() []Category {
	return []Category{
		CategorySystem, CategoryFile, CategoryDev, CategoryNetwork,
		CategoryText, CategoryMedia, CategoryProductivity,
	}
}

// QuickAction is something a tool can do straight from the command palette
// or the tray, such as "Keep awake for 1 hour".
type QuickAction struct {
	ID       string
	Title    Message
	Short    Message       // a name that fits a chip, such as "1 hour"; nil when the action is none
	Length   time.Duration // how long it keeps the tool on; 0 when it is no length
	Tray     bool          // shows as a chip in the tray, which has room for a few
	Shortcut string        // an accelerator to show, such as "Alt+1"
	Stops    bool          // turns the tool off, rather than starting a mode
	Run      func(ctx context.Context) error
}

// Manifest describes a tool. The UI is generated from it.
type Manifest struct {
	ID        string
	Name      Message
	Summary   Message
	Category  Category
	Kind      Kind
	Icon      string // a Material Symbols glyph name, in snake_case
	Art       string // an icon in design/assets/tool-icons, when the tool has one
	Keywords  []string
	Actions   []QuickAction
	Platforms []string
	Soon      bool // listed, but not usable yet
}

// Tool is anything with a manifest and declared actions.
type Tool interface {
	Manifest() Manifest
	// Specs declares the tool's actions; every front end is generated
	// from them (docs/agents.md section 1).
	Specs() []Spec
}

// Status is what a switch tool reports.
type Status struct {
	On     bool
	Text   string    // "Running · 2:29:10 left"
	Detail string    // "Display stays on"
	Active string    // the ID of the quick action in effect; empty when off
	Since  time.Time // when the run in Active began; zero when off
	Until  time.Time // when the run in Active ends by itself; zero when off or until turned off
	Note   string    // another thing that holds the tool on, such as "Plan also holds it until 18:00"
}

// Clock writes a duration as 2:29:10.
func Clock(d time.Duration) string {
	return fmt.Sprintf("%d:%02d:%02d", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60)
}

// ActivePlan is the Active of a Status held on by the tool's plan, not by
// a quick action.
const ActivePlan = "plan"

// Switch is a tool that keeps running in the background once started.
type Switch interface {
	Tool
	Start(ctx context.Context) error
	Stop() error
	Status() Status
}

// Timed is a switch tool that can also run for a length the caller picks,
// rather than only with the settings used last. The caller works the
// length out with ParseHold.
type Timed interface {
	Switch
	StartFor(ctx context.Context, d time.Duration) error
}

// Screened is a switch tool that can keep the display on too, which a
// person chooses apart from how long it runs: a mode keeps the display on or
// not as that choice says. A change reaches the run in progress.
type Screened interface {
	Switch
	Display() bool
	SetDisplay(on bool)
}

// Scheduled is a switch tool that turns itself on and off by a plan. A
// tool turned off by hand stays off for the stretch of the plan it was
// turned off in.
type Scheduled interface {
	Switch
	Schedule() Schedule
	SetSchedule(Schedule)
}

// Action is a tool that runs once: Plan works out what would happen and
// changes nothing; Apply does exactly what a plan lists. The input of Plan
// is described by the input schema of its "plan" spec.
type Action interface {
	Tool
	Plan(ctx context.Context, in Input) (Plan, error)
	Apply(ctx context.Context, p Plan) (Result, error)
}

// Input holds the values of a plan's input, by property name.
type Input map[string]string

// Tone colors a row of a plan.
type Tone int

// The tones of rows.
const (
	ToneNormal Tone = iota
	ToneSuccess
	ToneWarning
	ToneDanger
)

// Process names an executable for a row's process icon: its path, and
// whether the tool classes it as a system or protected service.
type Process struct {
	Path   string
	System bool
}

// Row is one line of a plan: what would happen to one thing.
type Row struct {
	Title   string
	Detail  string
	Cells   []string // columns of a DataTable: port, process, pid, address…
	Value   string   // copied when the row is clicked, if set
	Tone    Tone
	Process *Process // the executable of the row, when it names a process
}

// Plan is what an action tool would do. ApplyLabel is empty for tools that
// only show information.
type Plan struct {
	Summary     string
	Columns     []string // column headers when Rows carry Cells
	Rows        []Row
	ApplyLabel  string
	Destructive bool
	Data        any // the tool's own description of the work, for Apply
}

// Result is what Apply did.
type Result struct {
	Summary string
}

// Problem is an error caused by what someone typed or chose, with a message
// in the language shown, which the UI shows as it is.
type Problem struct {
	Text string
	Err  error
}

func (p *Problem) Error() string { return p.Err.Error() }

func (p *Problem) Unwrap() error { return p.Err }
