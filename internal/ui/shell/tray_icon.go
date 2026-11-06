package shell

import (
	"bytes"
	"image/png"
	"runtime"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// The design system's TrayIcon: four toys, outlined, and filled while any
// tool runs.
const (
	trayOutline = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="none" stroke="#000" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><circle cx="6" cy="6" r="2.9"/><rect x="11.1" y="3.1" width="5.8" height="5.8" rx="1.4"/><path d="M6 11.2l3 5.4H3z"/><path d="M14 11v5.8M11.1 13.9h5.8"/></svg>`
	trayFilled  = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="none" stroke="#000" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><circle cx="6" cy="6" r="2.9" fill="#000"/><rect x="11.1" y="3.1" width="5.8" height="5.8" rx="1.4" fill="#000"/><path d="M6 11.2l3 5.4H3z" fill="#000"/><path d="M14 11v5.8M11.1 13.9h5.8"/></svg>`
)

// trayIconLook is what the tray icon shows.
type trayIconLook struct {
	filled bool // a tool runs
	white  bool // for a dark taskbar (not macOS, which tints templates)
}

// trayIconState caches the icon's PNGs and remembers the one set.
type trayIconState struct {
	pngs map[trayIconLook][]byte
	set  *trayIconLook
}

// png draws an icon with MyGo's own renderer at 2× (a 16-point icon as 32
// pixels), black for macOS to tint as a template, white or black elsewhere.
func (s *trayIconState) png(look trayIconLook) []byte {
	if b, ok := s.pngs[look]; ok {
		return b
	}
	src := trayOutline
	if look.filled {
		src = trayFilled
	}
	svg := ui.MustParseSVG([]byte(src))
	ink := ui.RGB(0, 0, 0)
	if look.white {
		ink = ui.RGB(255, 255, 255)
	}
	img := ui.Render(func(c *ui.Context) {
		c.Root().Background(ui.Transparent)
		ui.Box(c).Fill().Center().Children(func() {
			ui.Image(c, svg).Size(ds.TrayIconSize, ds.TrayIconSize).TextColor(ink)
		})
	}, ds.TrayIconSize, ds.TrayIconSize, ds.TrayIconScale)
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	if s.pngs == nil {
		s.pngs = map[trayIconLook][]byte{}
	}
	s.pngs[look] = b.Bytes()
	return b.Bytes()
}

func (a *App) trayLook() trayIconLook {
	return trayIconLook{
		filled: len(a.reg.Running()) > 0,
		white:  runtime.GOOS != goosMac && a.tray != nil && a.tray.Dark(),
	}
}

func (a *App) updateTrayIcon() {
	if a.tray == nil {
		return
	}
	look := a.trayLook()
	if a.trayIcon.set != nil && *a.trayIcon.set == look {
		return
	}
	a.trayIcon.set = &look
	_ = a.tray.SetIcon(a.trayIcon.png(look), runtime.GOOS == goosMac)
}
