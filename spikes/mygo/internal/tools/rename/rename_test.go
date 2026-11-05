package rename

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cxxzsh/oh-my-toys/spikes/mygo/internal/tool"
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
