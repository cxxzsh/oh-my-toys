// Package dupes finds files with the same content in a folder, and moves
// the extra copies to the Trash.
package dupes

import (
	"cmp"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/platform/trash"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// ID is the tool's ID and command name.
const ID = "dupes"

const fieldFolder = "folder"

// Group is files of the same content; the first is the one kept.
type Group struct {
	Size  int64
	Paths []string
}

// Tool is Find Duplicates.
type Tool struct{}

// New makes the tool.
func New() *Tool { return &Tool{} }

// Manifest describes the tool.
func (*Tool) Manifest() tool.Manifest {
	return tool.Manifest{
		ID:        ID,
		Name:      i18n.Messages.DupesName,
		Summary:   i18n.Messages.DupesSummary,
		Category:  tool.CategoryFile,
		Kind:      tool.KindAction,
		Icon:      "files",
		Keywords:  []string{"duplicates", "dupes", "same files", "重复", "去重"},
		Platforms: []string{"macos", "windows", "linux"},
	}
}

// Specs declares the tool's actions: a plan that finds the duplicates and
// changes nothing, and an apply that moves the extra copies to the Trash.
func (*Tool) Specs() []tool.Spec {
	folder := tool.Schema{
		Type: tool.TypeString, Format: tool.FormatPath,
		Title: i18n.Messages.FieldFolder, Hint: i18n.Messages.FieldFolderHint,
	}
	return []tool.Spec{
		tool.PlanSpec(i18n.Messages.DupesPlanSummary, tool.Schema{
			Type: tool.TypeObject, Properties: []tool.Property{{Name: fieldFolder, Schema: folder, Required: true}},
		}),
		tool.ApplySpec(i18n.Messages.DupesApplySummary, tool.RiskWrite),
	}
}

// Plan finds the groups of identical files under the folder: files of the
// same size, then of the same SHA-256. It keeps the first path of each
// group and would move the others to the Trash.
func (*Tool) Plan(ctx context.Context, in tool.Input) (tool.Plan, error) {
	dir := in[fieldFolder]
	if dir == "" {
		return tool.Plan{}, nil
	}
	groups, err := Find(ctx, dir)
	if err != nil {
		return tool.Plan{}, err
	}
	var extra int
	var freed int64
	rows := make([]tool.Row, 0, len(groups))
	for _, g := range groups {
		extra += len(g.Paths) - 1
		freed += g.Size * int64(len(g.Paths)-1)
		rows = append(rows, groupRows(dir, g)...)
	}
	p := tool.Plan{Summary: i18n.DupesFound(len(groups), Size(freed)), Rows: rows, Data: groups}
	if extra > 0 {
		p.ApplyLabel = i18n.DupesApply(extra)
		p.Destructive = true
	}
	return p, nil
}

func groupRows(dir string, g Group) []tool.Row {
	rows := make([]tool.Row, 0, len(g.Paths))
	for i, path := range g.Paths {
		rel, _ := filepath.Rel(dir, path)
		r := tool.Row{Title: rel, Detail: i18n.DupesKeep(Size(g.Size)), Value: path, Tone: tool.ToneSuccess}
		if i > 0 {
			r.Detail, r.Tone = i18n.DupesTrash(Size(g.Size)), tool.ToneWarning
		}
		rows = append(rows, r)
	}
	return rows
}

// Apply moves every copy but the first of each group to the Trash.
func (*Tool) Apply(ctx context.Context, p tool.Plan) (tool.Result, error) {
	groups, _ := p.Data.([]Group)
	done := 0
	var errs []error
	for _, g := range groups {
		for _, path := range g.Paths[1:] {
			if ctx.Err() != nil {
				return tool.Result{Summary: i18n.DupesDone(done)}, ctx.Err()
			}
			if err := trash.Move(path); err != nil {
				errs = append(errs, err)
				continue
			}
			done++
		}
	}
	return tool.Result{Summary: i18n.DupesDone(done)}, errors.Join(errs...)
}

// Find returns the groups of identical files under dir, the largest first,
// leaving out hidden files and folders and empty files.
func Find(ctx context.Context, dir string) ([]Group, error) {
	bySize, err := sizes(ctx, dir)
	if err != nil {
		return nil, err
	}
	var groups []Group
	for size, paths := range bySize {
		if len(paths) < 2 {
			continue
		}
		same, err := byHash(ctx, paths)
		if err != nil {
			return nil, err
		}
		for _, ps := range same {
			slices.Sort(ps)
			groups = append(groups, Group{Size: size, Paths: ps})
		}
	}
	slices.SortFunc(groups, func(a, b Group) int {
		return cmp.Or(cmp.Compare(b.Size, a.Size), strings.Compare(a.Paths[0], b.Paths[0]))
	})
	return groups, nil
}

func sizes(ctx context.Context, dir string) (map[int64][]string, error) {
	bySize := map[int64][]string{}
	if err := filepath.WalkDir(dir, visit(ctx, dir, bySize)); err != nil {
		return nil, fmt.Errorf("walk %s: %w", dir, err)
	}
	return bySize, nil
}

// visit adds each regular, non-empty, visible file to bySize, skipping
// hidden folders and entries that cannot be read.
func visit(ctx context.Context, root string, bySize map[int64][]string) fs.WalkDirFunc {
	return func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // an unreadable entry is skipped, not fatal
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if path != root && strings.HasPrefix(d.Name(), ".") {
			return skipHidden(d)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if info, err := d.Info(); err == nil && info.Size() > 0 {
			bySize[info.Size()] = append(bySize[info.Size()], path)
		}
		return nil
	}
}

func skipHidden(d fs.DirEntry) error {
	if d.IsDir() {
		return filepath.SkipDir
	}
	return nil
}

func byHash(ctx context.Context, paths []string) ([][]string, error) {
	byHash := map[[sha256.Size]byte][]string{}
	for _, p := range paths {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		sum, err := hashFile(p)
		if err != nil {
			continue // a file that cannot be read is no duplicate
		}
		byHash[sum] = append(byHash[sum], p)
	}
	var out [][]string
	for _, ps := range byHash {
		if len(ps) > 1 {
			out = append(out, ps)
		}
	}
	return out, nil
}

func hashFile(path string) ([sha256.Size]byte, error) {
	var sum [sha256.Size]byte
	f, err := os.Open(path) //nolint:gosec // G304: reading the files of the folder the person chose is the point
	if err != nil {
		return sum, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }() // a file only read has nothing to lose on close
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return sum, fmt.Errorf("read %s: %w", path, err)
	}
	copy(sum[:], h.Sum(nil))
	return sum, nil
}

// Units of sizes, by powers of 1024.
const (
	kib = 1 << 10
	mib = 1 << 20
	gib = 1 << 30
)

// Size writes a number of bytes in the language shown: "1.2 MB".
func Size(n int64) string {
	switch {
	case n >= gib:
		return i18n.SizeGB(float64(n) / gib)
	case n >= mib:
		return i18n.SizeMB(float64(n) / mib)
	case n >= kib:
		return i18n.SizeKB(float64(n) / kib)
	}
	return i18n.SizeBytes(int(n))
}
