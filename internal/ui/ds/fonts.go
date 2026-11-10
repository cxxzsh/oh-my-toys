package ds

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/design"
)

// The two bundled families, by the names the font stacks of the tokens use,
// and their files in the design folder.
const (
	familySans = "IBM Plex Sans"
	fileSans   = "fonts/IBMPlexSans-Variable.ttf"
	familyMono = "IBM Plex Mono"
	fileMono   = "fonts/IBMPlexMono-Variable.ttf"
)

// RegisterFonts makes the bundled fonts, IBM Plex Sans and IBM Plex Mono,
// usable by every window. The app calls it once at start, before its first
// window.
func RegisterFonts() error {
	if err := registerFont(familySans, fileSans); err != nil {
		return err
	}
	return registerFont(familyMono, fileMono)
}

func registerFont(family, file string) error {
	data, err := design.Read(file)
	if err != nil {
		return err
	}
	if err := ui.RegisterFont(data, family); err != nil {
		return fmt.Errorf("register font %s: %w", family, err)
	}
	return nil
}
