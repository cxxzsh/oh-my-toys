// Package design embeds the design system's fonts and image assets, so the
// app carries them and never loads anything from the network. Names are
// relative to the design folder: "fonts/Geist-Variable.ttf",
// "assets/glyphs/search.svg".
package design

import (
	"embed"
	"fmt"
	"path"
)

// root is the folder of the design system inside this directory.
const root = "next"

//go:embed next/fonts/Geist-Variable.ttf next/fonts/JetBrainsMono-Variable.ttf
var files embed.FS

// Read returns an embedded file of the design system.
func Read(name string) ([]byte, error) {
	b, err := files.ReadFile(path.Join(root, name))
	if err != nil {
		return nil, fmt.Errorf("design asset %s: %w", name, err)
	}
	return b, nil
}
