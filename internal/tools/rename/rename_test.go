package rename

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

func TestPlanThenApply(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"IMG_1.jpg", "IMG_2.jpg", "photo_2.jpg", ".hidden_IMG"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	tl := New()
	in := tool.Input{fieldFolder: dir, fieldFind: "IMG_", fieldReplace: "photo_"}
	p, err := tl.Plan(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	changes, _ := p.Data.([]Change)
	if len(changes) != 1 || filepath.Base(changes[0].To) != "photo_1.jpg" {
		t.Fatalf("changes %v", changes)
	}
	if _, err := os.Stat(filepath.Join(dir, "IMG_1.jpg")); err != nil {
		t.Fatal("Plan changed a file")
	}
	if _, err := tl.Apply(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"photo_1.jpg", "IMG_2.jpg", "photo_2.jpg"} {
		if _, err := os.Stat(filepath.Join(dir, n)); err != nil {
			t.Errorf("%s: %v", n, err)
		}
	}
}

// snapshot lists a folder's files and their contents.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(path)
		out[path] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestRiskMatchesBehaviour checks the declared risks: the plan, declared
// read, leaves the folder as it was; the apply, declared write, changes it.
func TestRiskMatchesBehaviour(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a-old.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	risks := map[string]tool.Risk{}
	for _, s := range New().Specs() {
		risks[s.Name] = s.Risk
	}
	before := snapshot(t, dir)
	p, err := New().Plan(context.Background(), tool.Input{fieldFolder: dir, fieldFind: "old", fieldReplace: "new"})
	if err != nil {
		t.Fatal(err)
	}
	if risks[tool.ActionPlan] != tool.RiskRead || len(snapshot(t, dir)) != len(before) || snapshot(t, dir)[filepath.Join(dir, "a-old.txt")] != "x" {
		t.Error("the plan is read-only and leaves the folder as it was")
	}
	if _, err := New().Apply(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if _, renamed := snapshot(t, dir)[filepath.Join(dir, "a-new.txt")]; risks[tool.ActionApply] != tool.RiskWrite || !renamed {
		t.Error("the apply is declared write and changes the folder")
	}
}
