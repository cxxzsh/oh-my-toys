// Package tool is the contract every tool implements: a manifest, and
// Start/Stop/Status for switch tools or Plan/Apply for action tools. It
// sits below the tools, which implement it, and the core, which runs them.
package tool

import (
	"context"

	"github.com/cxxzsh/oh-my-toys/spikes/mygo/internal/i18n"
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
	Shortcut string // an accelerator to show, such as "Alt+1"
	Run      func(ctx context.Context) error
}

// Manifest describes a tool. The UI is generated from it.
type Manifest struct {
	ID        string
	Name      Message
	Summary   Message
	Category  Category
	Kind      Kind
	Icon      string // a Lucide icon name
	Keywords  []string
	Actions   []QuickAction
	Platforms []string
	Soon      bool // listed, but not usable yet
}

// Tool is anything with a manifest.
type Tool interface {
	Manifest() Manifest
}

// Status is what a switch tool reports.
type Status struct {
	On     bool
	Text   string // "Running · 2:29:10 left"
	Detail string // "Display stays on"
}

// Switch is a tool that keeps running in the background once started.
type Switch interface {
	Tool
	Start(ctx context.Context) error
	Stop() error
	Status() Status
}

// Action is a tool that runs once: Plan works out what would happen and
// changes nothing; Apply does exactly what a plan lists.
type Action interface {
	Tool
	Fields() []Field
	Plan(ctx context.Context, in Input) (Plan, error)
	Apply(ctx context.Context, p Plan) (Result, error)
}

// FieldKind says how the UI asks for a field.
type FieldKind int

// The kinds of fields.
const (
	FieldText FieldKind = iota
	FieldFolder
)

// Field is an input an action tool asks for.
type Field struct {
	ID          string
	Label       Message
	Placeholder Message
	Kind        FieldKind
}

// Input holds the values of an action tool's fields, by field ID.
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

// Row is one line of a plan: what would happen to one thing.
type Row struct {
	Title  string
	Detail string
	Value  string // copied when the row is clicked, if set
	Tone   Tone
}

// Plan is what an action tool would do. ApplyLabel is empty for tools that
// only show information.
type Plan struct {
	Summary     string
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
