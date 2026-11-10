// Package design embeds the design system's fonts and image assets, so the
// app carries them and never loads anything from the network. Names are
// relative to the design folder: "fonts/IBMPlexSans-Variable.ttf",
// "assets/glyphs/search.svg".
package design

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

//go:embed fonts/IBMPlexSans-Variable.ttf fonts/IBMPlexMono-Variable.ttf
//go:embed assets/glyphs/*.svg assets/tray-icon/*.svg assets/process-icon/*.svg
//go:embed assets/app-icon/*-128.png
var files embed.FS

// Read returns an embedded file of the design system.
func Read(name string) ([]byte, error) {
	b, err := files.ReadFile(path.Clean(name))
	if err != nil {
		return nil, fmt.Errorf("design asset %s: %w", name, err)
	}
	return b, nil
}

// Names lists the files of an embedded folder, such as "assets/glyphs",
// without their extension.
func Names(dir string) ([]string, error) {
	entries, err := fs.ReadDir(files, path.Clean(dir))
	if err != nil {
		return nil, fmt.Errorf("design folder %s: %w", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, strings.TrimSuffix(e.Name(), path.Ext(e.Name())))
	}
	return names, nil
}
