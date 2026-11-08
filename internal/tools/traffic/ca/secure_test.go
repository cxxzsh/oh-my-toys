package ca

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRemoveDeletesTheKeyFirstAndTemps checks that a failure to delete the
// certificate does not leave the key behind, and that leftover temporary
// files go too.
func TestRemoveDeletesTheKeyFirstAndTemps(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ca")
	if _, err := Create(dir); err != nil {
		t.Fatal(err)
	}
	temp := filepath.Join(dir, rootKeyFile+".tmp")
	if err := os.WriteFile(temp, []byte("leftover"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A non-empty folder where the certificate should be makes its removal
	// fail, which must not stop the key from going.
	certPath := filepath.Join(dir, rootCertFile)
	if err := os.Remove(certPath); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(certPath, "sub"), dirMode); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(certPath, "sub", "keep"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Remove(dir); err == nil {
		t.Fatal("Remove with an undeletable certificate = nil, want an error")
	}
	if _, err := os.Stat(filepath.Join(dir, rootKeyFile)); !errors.Is(err, fs.ErrNotExist) {
		t.Error("the key survived a failed Remove")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Errorf("temporary file %q survived Remove", entry.Name())
		}
	}
}

// TestCreateTightensAnExistingFolder checks that a folder another program
// made wider ends up private.
func TestCreateTightensAnExistingFolder(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ca")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(dir); err != nil {
		t.Fatal(err)
	}
	checkMode(t, dir, dirMode)
}

// TestCreateDoesNotWriteThroughAHostileTempFile checks that a symlink left
// at a temporary name cannot make Create write where it points.
func TestCreateDoesNotWriteThroughAHostileTempFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ca")
	if err := os.MkdirAll(dir, dirMode); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("untouched"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, rootCertFile+".tmp")); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(dir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "untouched" {
		t.Fatalf("Create wrote through the symlink: %q", data)
	}
}
