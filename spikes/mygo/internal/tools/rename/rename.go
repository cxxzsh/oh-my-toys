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

	"github.com/cxxzsh/oh-my-toys/spikes/mygo/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/spikes/mygo/internal/tool"
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

// Fields asks for a folder, the text to find and its replacement.
func (*Tool) Fields() []tool.Field {
	return []tool.Field{
		{ID: fieldFolder, Label: i18n.Messages.FieldFolder, Placeholder: i18n.Messages.FieldFolderHint, Kind: tool.FieldFolder},
		{ID: fieldFind, Label: i18n.Messages.RenameFind, Placeholder: i18n.Messages.RenameFindHint},
		{ID: fieldReplace, Label: i18n.Messages.RenameReplace, Placeholder: i18n.Messages.RenameReplaceHint},
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
