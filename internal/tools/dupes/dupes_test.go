package dupes

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

func TestFind(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("a.txt", "same")
	write("sub/b.txt", "same")
	write("c.txt", "diff")
	write(".git/d.txt", "same")
	write("empty1", "")
	write("empty2", "")
	groups, err := Find(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || len(groups[0].Paths) != 2 {
		t.Fatalf("groups %+v", groups)
	}
}

// TestPlanChangesNothing checks the plan's declared risk, read: the folder
// is the same after it, duplicates included.
func TestPlanChangesNothing(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"a", "b"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("same"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	p, err := New().Plan(context.Background(), tool.Input{fieldFolder: dir})
	if err != nil || p.ApplyLabel == "" {
		t.Fatalf("plan %+v, %v", p, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 2 {
		t.Errorf("the folder changed: %v %v", entries, err)
	}
	for _, s := range New().Specs() {
		if s.Name == tool.ActionPlan && s.Risk != tool.RiskRead {
			t.Errorf("plan risk %q", s.Risk)
		}
		if s.Name == tool.ActionApply && s.Risk != tool.RiskWrite {
			t.Errorf("apply risk %q: it moves files to the Trash", s.Risk)
		}
	}
}
