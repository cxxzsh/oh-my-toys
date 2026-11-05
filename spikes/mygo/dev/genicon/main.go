// Command genicon draws the app icon of the design system (a white rounded
// square, the four toys in category colors) at 1024×1024 into
// design/app-icon.png (the icon of mygo.json), with MyGo's own renderer.
//
//	go run ./dev/genicon
package main

import (
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"path/filepath"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/spikes/mygo/internal/ui/ds"
)

func main() {
	p := ds.Light
	toys := ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" stroke-linecap="round" stroke-linejoin="round">
<circle cx="6" cy="6" r="2.9" fill="` + hex(p.CatOrange) + `"/>
<rect x="11.1" y="3.1" width="5.8" height="5.8" rx="1.4" fill="` + hex(p.CatBlue) + `"/>
<path d="M6 11.2l3 5.4H3z" fill="` + hex(p.CatGreen) + `" stroke="` + hex(p.CatGreen) + `" stroke-width="1.2"/>
<path d="M14 11v5.8M11.1 13.9h5.8" fill="none" stroke="` + hex(p.CatPurple) + `" stroke-width="1.8"/>
</svg>`))
	img := ui.Render(func(c *ui.Context) {
		c.Root().Background(ui.Transparent)
		ui.Box(c).Fill().Center().Children(func() {
			// macOS's icon grid: an 824-point squircle in the 1024 canvas.
			ds.ApplyShadow(ui.Box(c).Size(824, 824).Radius(185).Background(p.BgCard), []ds.Shadow{{X: 0, Y: 10, Blur: 24, Color: ui.RGBA(0, 0, 0, 0.18)}}).
				Border(2, p.BorderCard).Center().Children(func() {
				ui.Image(c, toys).Size(560, 560)
			})
		})
	}, 1024, 1024, 1)
	if err := save("design/app-icon.png", img); err != nil {
		log.Fatal(err)
	}
}

func save(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	f, err := os.Create(path) //nolint:gosec // G304: the path is a constant of this generator
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		return fmt.Errorf("encode %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func hex(c ui.Color) string {
	const digits = "0123456789abcdef"
	b := []byte{'#', 0, 0, 0, 0, 0, 0}
	for i, v := range []uint8{c.R, c.G, c.B} {
		b[1+2*i], b[2+2*i] = digits[v>>4], digits[v&15]
	}
	return string(b)
}
