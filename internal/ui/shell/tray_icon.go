package shell

import (
	"bytes"
	"image/png"
	"runtime"

	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/design"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// trayIconLook is what the tray icon shows: the four shapes of the app
// icon, with the circle filled while any switch tool is on, in black or
// white by the tray's theme.
type trayIconLook struct {
	active bool // a switch tool runs
	white  bool // for a dark taskbar (not macOS, which tints templates)
}

type trayIconState struct {
	pngs map[trayIconLook][]byte
	set  *trayIconLook
}

func (l trayIconLook) file() string {
	switch {
	case l.white && l.active:
		return "assets/tray-icon/tray-white-active.svg"
	case l.white:
		return "assets/tray-icon/tray-white.svg"
	case l.active:
		return "assets/tray-icon/tray-black-active.svg"
	default:
		return "assets/tray-icon/tray-black.svg"
	}
}

func (s *trayIconState) png(look trayIconLook) []byte {
	if b, ok := s.pngs[look]; ok {
		return b
	}
	raw, err := design.Read(look.file())
	if err != nil {
		return nil
	}
	svg := ui.MustParseSVG(raw)
	size := ds.TrayIconTaskbar
	if runtime.GOOS == goosMac {
		size = ds.TrayIconTopBar
	}
	img := ui.Render(func(c *ui.Context) {
		c.Root().Background(ui.Transparent)
		ui.Box(c).Fill().Center().Children(func() {
			ui.Image(c, svg).Size(float32(size), float32(size))
		})
	}, size, size, ds.TrayIconScale)
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
		active: len(a.reg.Running()) > 0,
		white:  runtime.GOOS != goosMac && a.tray != nil && a.tray.Dark(),
	}
}

func (a *App) updateTrayIcon() {
	if a.tray == nil {
		return
	}
	a.tray.SetToolTip(a.trayTooltip())
	look := a.trayLook()
	if a.trayIcon.set != nil && *a.trayIcon.set == look {
		return
	}
	a.trayIcon.set = &look
	_ = a.tray.SetIcon(a.trayIcon.png(look), runtime.GOOS == goosMac)
}
