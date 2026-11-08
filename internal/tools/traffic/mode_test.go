package traffic

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// checkMode fails when path does not have want. Windows keeps access control
// lists, not mode bits: Stat reports 0666 and 0777 whatever the ACL says, so
// there is nothing to check there.
func checkMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("%s has mode %o, want %o", filepath.Base(path), got, want)
	}
}
