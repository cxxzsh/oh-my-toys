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
		ToolTip:        i18n.AppName(),
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

func (a *App) trayMenu() *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: i18n.TrayTitle(), Click: func(*mygo.MenuItem, *mygo.Window) { a.toggleTray() }},
		{Label: i18n.OpenMain(), Click: func(*mygo.MenuItem, *mygo.Window) { a.openMain(pageHome) }},
		mygo.Separator(),
		{Label: i18n.Quit(), Role: mygo.RoleQuit},
	})
}

// setTrayMenu gives the icon a menu on Linux, whose AppIndicator tells no
// clicks; on macOS and Windows a left click opens the panel and a right
// click the menu, made anew each time in the language shown.
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

// newTrayPanel makes the panel once, hidden: a click only shows it.
func (a *App) newTrayPanel() *mygo.Window {
	fl := mygo.NewFlyout(mygo.FlyoutOptions{
		Tray:      a.tray,
		Placement: mygo.PlacementBottom,
		Gap:       ds.TrayGap,
		Width:     ds.TrayWidth,
		Height:    ds.TrayFirstHeight,
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
	a.trayQuery = ""
	a.trayInput.Focus()
	a.panel.Show()
}

func (a *App) hideTray() {
	if a.panel != nil {
		a.panel.Hide()
	}
}

// trayView is the TrayPanel: search, what runs, the pinned tools, and the
// main window and quit at the bottom.
func (a *App) trayView(c *ui.Context) {
	a.theme(c)
	p := ds.P(c)
	c.Root().Background(ui.Transparent)
	if c.Shortcut(ui.Cmd, ui.KeyO) {
		a.hideTray()
		a.openMain(pageHome)
	}
	panel := ui.Column(c).FillWidth().Radius(ds.RadiusXl).Background(p.BgElevated).Padding(ds.Space3).Gap(ds.Space3)
	if runtime.GOOS == goosLinux {
		ds.ApplyShadow(panel, ds.S(c).ShadowPopover) // macOS and Windows draw the system's shadow
	}
	panel.Children(func() {
		a.trayHead(c)
		if a.trayQuery != "" {
			a.trayResults(c)
		} else {
			a.trayRunning(c)
			a.trayLaunch(c)
		}
		ui.Box(c).FillWidth().Height(1).Margin(0, ds.TrayInset).Background(p.Separator)
		ui.Row(c).FillWidth().Padding(0, ds.TrayFootPad).Gap(ds.TrayFootPad).Children(func() {
			a.trayItem(c, "layout-grid", i18n.OpenMain(), modKey("O"), true, func() { a.hideTray(); a.openMain(pageHome) })
			a.trayItem(c, "log-out", i18n.Quit(), "", false, mygo.App.Quit)
		})
	}).DrawOver(func(*ui.Painter, ui.Rect) { a.metrics.trayPainted() })
	fitHeight(c, a.panel, panel)
}

func (a *App) trayHead(c *ui.Context) {
	p := ds.P(c)
	ui.Row(c).FillWidth().Padding(ds.Space1, ds.TrayInset, 0).Gap(ds.Space2).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Grow(1).Children(func() {
			a.searchField(c, &a.trayQuery, &a.trayInput, i18n.SearchNTools(len(a.reg.All())), "")
		})
		set := ui.ButtonBase(c).Label(i18n.Settings())
		if set.Clicked() {
			a.hideTray()
			a.openMain(pageSettings)
		}
		set.Children(func() { comp.Glyph(c, "settings", ds.Glyph, p.TextSecondary) })
	})
}

func (a *App) trayItem(c *ui.Context, icon, label, hint string, grow bool, fn func()) {
	p := ds.P(c)
	b := ui.ButtonBase(c.Key(label)).Label(label)
	if b.Clicked() {
		fn()
	}
	bg := ui.Transparent
	if b.Hovered() {
		bg = p.FillHover
	}
	if grow {
		b.Grow(1).Basis(0)
	} else {
		b.Shrink(0)
	}
	b.Height(ds.TrayItemHeight).Padding(0, ds.TrayInset).Gap(ds.Space2).Radius(ds.RadiusSm).Background(bg).
		AlignItems(ui.Center).Children(func() {
		comp.Glyph(c, icon, ds.Glyph, p.TextSecondary)
		comp.Text(c, label, ds.Body, p.TextPrimary).NoWrap()
		if grow {
			ui.Spacer(c)
		}
		if hint != "" {
			comp.Text(c, hint, ds.Callout, p.TextTertiary)
		}
	})
}

// fitHeight resizes a window to the height its content laid out at.
// MyGo v0.7.4 fits only web pages to their content (FitContent); its next
// version does native UI too, which would replace this.
func fitHeight(c *ui.Context, w *mygo.Window, content ui.Element) {
	if w == nil {
		return
	}
	b := content.Bounds()
	if b.H <= 0 {
		return
	}
	cw, ch := w.ContentSize()
	if h := int(b.H + 0.5); h != ch {
		w.SetContentSize(cw, h)
		c.Invalidate()
	}
}
