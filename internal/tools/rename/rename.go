// Package rename renames the files of a folder by replacing text in their
// names, after showing what would change.
package rename

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// ID is the tool's ID and command name.
const ID = "rename"

// Field IDs.
const (
	fieldFolder  = "folder"
	fieldFind    = "find"
	fieldReplace = "replace"
)

// Change is one file to rename.
type Change struct {
	From, To string // full paths
}

// Tool is Batch Rename.
type Tool struct{}

// New makes the tool.
func New() *Tool { return &Tool{} }

// Manifest describes the tool.
func (*Tool) Manifest() tool.Manifest {
	return tool.Manifest{
		ID:        ID,
		Name:      i18n.Messages.RenameName,
		Summary:   i18n.Messages.RenameSummary,
		Category:  tool.CategoryFile,
		Kind:      tool.KindAction,
		Icon:      "text-cursor-input",
		Keywords:  []string{"rename", "batch rename", "replace", "改名"},
		Platforms: []string{"macos", "windows", "linux"},
	}
}

// Specs declares the tool's actions: a plan that lists the new names and
// changes nothing, and an apply that renames the files it lists.
func (*Tool) Specs() []tool.Spec {
	str := func(title, hint tool.Message) tool.Schema {
		return tool.Schema{Type: tool.TypeString, Title: title, Hint: hint}
	}
	folder := str(i18n.Messages.FieldFolder, i18n.Messages.FieldFolderHint)
	folder.Format = tool.FormatPath
	return []tool.Spec{
		tool.PlanSpec(i18n.Messages.RenamePlanSummary, tool.Schema{Type: tool.TypeObject, Properties: []tool.Property{
			{Name: fieldFolder, Schema: folder, Required: true},
			{Name: fieldFind, Schema: str(i18n.Messages.RenameFind, i18n.Messages.RenameFindHint), Required: true},
			{Name: fieldReplace, Schema: str(i18n.Messages.RenameReplace, i18n.Messages.RenameReplaceHint)},
		}}),
		tool.ApplySpec(i18n.Messages.RenameApplySummary, tool.RiskWrite),
	}
}

// Plan lists the files of the folder whose names contain the text, with
// their new names. A name that would clash with another file is shown and
// left out. Plan changes nothing.
func (*Tool) Plan(ctx context.Context, in tool.Input) (tool.Plan, error) {
	dir, find := in[fieldFolder], in[fieldFind]
	if dir == "" || find == "" {
		return tool.Plan{}, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return tool.Plan{}, fmt.Errorf("read %s: %w", dir, err)
	}
	changes, rows := plan(dir, entries, find, in[fieldReplace])
	if err := ctx.Err(); err != nil {
		return tool.Plan{}, err
	}
	p := tool.Plan{Summary: i18n.RenamePlanned(len(changes)), Rows: rows, Data: changes}
	if len(changes) > 0 {
		p.ApplyLabel = i18n.RenameApply(len(changes))
	}
	return p, nil
}

func plan(dir string, entries []os.DirEntry, find, replace string) ([]Change, []tool.Row) {
	taken := map[string]bool{}
	for _, e := range entries {
		taken[e.Name()] = true
	}
	var (
		changes []Change
		rows    []tool.Row
	)
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || !strings.Contains(name, find) {
			continue
		}
		to := strings.ReplaceAll(name, find, replace)
		if to == name {
			continue
		}
		if to == "" || taken[to] {
			rows = append(rows, tool.Row{Title: name, Detail: i18n.RenameConflict(to), Tone: tool.ToneDanger})
			continue
		}
		taken[to] = true
		changes = append(changes, Change{From: filepath.Join(dir, name), To: filepath.Join(dir, to)})
		rows = append(rows, tool.Row{Title: name, Detail: "→ " + to})
	}
	return changes, rows
}

// Apply renames the files the plan lists, and only those, checking again
// that no file took a new name meanwhile.
func (*Tool) Apply(ctx context.Context, p tool.Plan) (tool.Result, error) {
	changes, _ := p.Data.([]Change)
	done := 0
	var errs []error
	for _, c := range slices.Clone(changes) {
		if ctx.Err() != nil {
			break
		}
		if _, err := os.Lstat(c.To); err == nil {
			errs = append(errs, fmt.Errorf("rename %s: %w", c.From, os.ErrExist))
			continue
		}
		if err := os.Rename(c.From, c.To); err != nil {
			errs = append(errs, err)
			continue
		}
		done++
	}
	return tool.Result{Summary: i18n.RenameDone(done)}, errors.Join(errs...)
}
