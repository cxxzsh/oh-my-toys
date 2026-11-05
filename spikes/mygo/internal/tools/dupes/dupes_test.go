package dupes

import (
	"context"
	"os"
	"path/filepath"
	"testing"
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
