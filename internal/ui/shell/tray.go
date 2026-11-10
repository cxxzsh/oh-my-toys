package shell

import (
	"log"
	"runtime"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// trayClickGuard ignores a click on the icon that comes right after the
// press closed the panel: that click meant "close".
const trayClickGuard = 400 * time.Millisecond

func (a *App) setUpTray() {
	look := trayIconLook{white: runtime.GOOS != goosMac}
	tray, err := mygo.NewTray(mygo.TrayOptions{
		ID:             "oh-my-toys",
		Icon:           a.trayIcon.png(look),
		IconIsTemplate: runtime.GOOS == goosMac,
		ToolTip:        a.trayTooltip(),
	})
	if err != nil {
		log.Print("tray: ", err)
		return
	}
	a.tray = tray
	a.trayIcon.set = &look
	a.tray.OnAppearanceChange(func() { a.trayIcon.set = nil; a.updateTrayIcon() })
	a.updateTrayIcon()
	if runtime.GOOS != goosLinux {
		a.tray.OnClick(a.toggleTray)
		a.tray.OnRightClick(a.trayRightClick)
	}
	a.setTrayMenu()
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) { a.tray.Destroy() })
	a.panel = a.newTrayPanel()
}

func (a *App) trayTooltip() string {
	running := a.reg.Running()
	if len(running) == 0 {
		return i18n.AppName()
	}
	return i18n.TrayActiveTooltip(i18n.AppName(), running[0].Manifest().Name(msgs()))
}

func (a *App) trayMenu() *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: i18n.OpenApp(), Click: func(*mygo.MenuItem, *mygo.Window) { a.openMain(pageHome) }},
		mygo.Separator(),
		{Label: i18n.Quit(), Role: mygo.RoleQuit},
	})
}

func (a *App) setTrayMenu() {
	if a.tray != nil && runtime.GOOS == goosLinux {
		a.tray.SetMenu(a.trayMenu())
	}
}

func (a *App) trayRightClick() {
	a.hideTray()
	a.tray.SetMenu(a.trayMenu())
	defer a.tray.SetMenu(nil)
	a.tray.PopUpMenu()
}

func (a *App) newTrayPanel() *mygo.Window {
	fl := mygo.NewFlyout(mygo.FlyoutOptions{
		Tray:      a.tray,
		Placement: mygo.PlacementBottomEnd,
		Gap:       ds.TrayGap,
		Width:     ds.TrayWidth,
		Height:    ds.TrayWidth,
		Focusable: true,
		Hidden:    true,
		Shadow:    true,
		Content:   ui.View(a.trayView),
	})
	fl.OnClose(func(e *mygo.CloseEvent) {
		if !a.quitting {
			e.PreventDefault()
			fl.Hide()
			a.trayHidden = time.Now()
		}
	})
	return fl
}

func (a *App) toggleTray() {
	if a.panel == nil {
		return
	}
	if a.panel.IsVisible() {
		a.hideTray()
		return
	}
	if time.Since(a.trayHidden) < trayClickGuard {
		return
	}
	a.metrics.trayClicked()
	a.trayChosen = 0
	a.placeTray()
	a.panel.Show()
}

// The panel only learns its height from the frames it draws, so the first
// time it opens it would show at the size it was made with and grow to its
// content a moment later: a flash. So until it has fitted once it draws
// nothing, though it is laid out. Later it keeps its height while closed, and
// shows as it was. A frame knows the size of the one before it, so fitting
// takes trayFramesToFit frames.
const trayFramesToFit = 2

// waitsForHeight reports whether the panel has yet to fit for the first time.
func (a *App) waitsForHeight() bool { return a.panel != nil && !a.trayRevealed }

// fitTray fits the panel to its content, and lets it draw once it fits.
func (a *App) fitTray(c *ui.Context, content ui.Element) {
	moved := fitHeight(c, a.panel, content)
	if !a.waitsForHeight() {
		return
	}
	c.Invalidate()
	if a.trayFrames++; !moved && a.trayFrames >= trayFramesToFit {
		a.trayRevealed = true
	}
}

// placeTray puts the panel against the icon as it is now. The panel was
// placed when it last changed size, and showing it does not place it again,
// but the icon moves as the other items of the menu bar change width, and
// with a second display the click may be on that display's copy of it.
// Setting the anchor places a tray's flyout again, though the anchor itself
// does not apply to it.
func (a *App) placeTray() {
	a.panel.SetAnchor(mygo.Rectangle{})
}

func (a *App) hideTray() {
	if a.panel != nil {
		a.panel.Hide()
	}
}

func (a *App) trayView(c *ui.Context) {
	a.theme(c)
	p := ds.P(c)
	c.Root().Background(ui.Transparent)
	items := a.trayItems()
	a.trayKeys(c, items)
	panel := ui.Column(c).FillWidth().Radius(ds.RadiusLg).Background(p.BgPopover).
		Border(ds.LineWidth, p.LineControl).Padding(ds.Space8)
	if runtime.GOOS == goosLinux {
		ds.ApplyShadow(panel, ds.S(c).ShadowPopover)
	}
	if a.waitsForHeight() {
		panel.Invisible()
	}
	panel.Children(func() {
		a.trayHead(c)
		a.traySearchTrigger(c)
		a.trayBody(c, items)
	}).DrawOver(func(*ui.Painter, ui.Rect) { a.metrics.trayPainted() })
	a.fitTray(c, panel)
}

func (a *App) trayHead(c *ui.Context) {
	p := ds.P(c)
	ui.Row(c).FillWidth().Padding(ds.Space4, ds.Space4, ds.Space8, ds.Space8).AlignItems(ui.Center).
		Children(func() {
			ui.Row(c).Gap(ds.Space8).AlignItems(ui.Center).Grow(1).Children(func() {
				a.kit.AppIcon(c, ds.TrayHeaderIcon)
				comp.Text(c, i18n.AppName(), ds.Headline, p.TextStrong).SingleLine()
			})
			ui.Row(c).Gap(ds.Space2).Children(func() {
				if a.kit.Button(c.Key("tray-set"), "", comp.ButtonOpts{
					Kind: comp.Plain, Glyph: glyphSettings, Tooltip: i18n.Settings(),
				}).Clicked() {
					a.hideTray()
					a.openMain(pageSettings)
				}
				if a.kit.Button(c.Key("tray-open"), "", comp.ButtonOpts{
					Kind: comp.Plain, Glyph: "open_in_new", Tooltip: i18n.OpenApp(),
				}).Clicked() {
					a.hideTray()
					a.openMain(pageHome)
				}
			})
		})
}

func (a *App) traySearchTrigger(c *ui.Context) {
	p := ds.P(c)
	b := ui.ButtonBase(c.Key("tray-search")).Label(i18n.PaletteHint()).Cursor(ui.CursorDefault)
	if b.Clicked() {
		a.hideTray()
		a.openMain("")
		a.showPalette()
	}
	fill := p.FillControl
	if b.Hovered() {
		fill = p.FillControlHover
	}
	b.FillWidth().Height(ds.ControlMd).Margin(0, ds.Space4, ds.Space6).Padding(0, ds.Space8).Gap(ds.Space6).
		Radius(ds.RadiusSm).Background(fill).Border(ds.LineWidth, p.LineControl).AlignItems(ui.Center).Children(func() {
		a.kit.Glyph(c, "search", ds.GlyphSm, p.TextMuted)
		comp.Text(c, i18n.PaletteHint(), ds.Caption, p.TextMuted).Grow(1).SingleLine()
		comp.Kbd(c, modKey("K"))
	})
}

// fitHeight gives a window the height of its content, and reports whether it
// had to: it is also true while the content has no size yet.
func fitHeight(c *ui.Context, w *mygo.Window, content ui.Element) (moved bool) {
	if w == nil {
		return false
	}
	b := content.Bounds()
	if b.H <= 0 {
		return true
	}
	cw, ch := w.ContentSize()
	if h := int(b.H + 0.5); h != ch {
		w.SetContentSize(cw, h)
		c.Invalidate()
		return true
	}
	return false
}
