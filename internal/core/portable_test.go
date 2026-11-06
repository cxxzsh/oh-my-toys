package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPortableDir(t *testing.T) {
	tests := []struct {
		name   string
		marker func(dir string) error // makes what sits at the marker's path
		want   bool
	}{
		{"no marker", func(string) error { return nil }, false},
		{"marker file", func(dir string) error {
			return os.WriteFile(filepath.Join(dir, PortableMarker), nil, 0o600)
		}, true},
		{"folder named like the marker", func(dir string) error {
			return os.Mkdir(filepath.Join(dir, PortableMarker), 0o700)
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := tt.marker(dir); err != nil {
				t.Fatal(err)
			}
			got, ok := PortableDir(filepath.Join(dir, "oh-my-toys.exe"))
			if ok != tt.want {
				t.Fatalf("portable = %v, want %v", ok, tt.want)
			}
			if ok && got != filepath.Join(dir, "data") {
				t.Errorf("data dir %q", got)
			}
			if !ok && got != "" {
				t.Errorf("data dir %q for an installed app", got)
			}
		})
	}
}
