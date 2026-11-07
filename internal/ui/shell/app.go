// Package shell is the desktop app around the tools: the main window
// (titlebar, sidebar, home page, tool pages, settings), the tray panel and
// the command palette. Everything it shows comes from the registry; it
// never names a tool.
package shell

import (
	"context"
	"log"
	"time"

	"github.com/egoist/mygo"
	mygoi18n "github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/core"
	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// Page names besides tool IDs.
const (
	pageHome      = "home"
	pageSettings  = "settings"
	glyphSettings = pageSettings
)

// tick is how often running switch tools redraw their countdowns.
const tick = time.Second

// Options configure the app.
type Options struct {
	Registry      *core.Registry
	Runner        *core.Runner
	Events        *core.Events
	Store         *core.Store
	Defaults      core.Settings // settings for a first launch
	Dev           bool          // show the measurements
	Portable      bool          // data next to the executable, no self-update
	Material      string        // main window: "sidebar", "glass", "mica", "none"
	Shortcuts     []string      // global shortcuts to try, in order
	ShowMain      bool
	StartedAt     time.Time
	OnFirstFrame  func(time.Duration)
	ScreenshotDir string
}

// App is the state of every window. Views read and change it on the main
// thread; other goroutines go through Post.
type App struct {
	opt      Options
	reg      *core.Registry
	kit      *comp.Kit
	procs    *comp.ProcessIcons
	settings core.Settings

	main  *mygo.Window
	tray  *mygo.Tray
	panel *mygo.Window

	page       string
	homeCat    tool.Category
	pageFilter string
	filterIn   ui.Handle
	sideHidden bool
	sideList   ui.ListState
	homeList   ui.ListState
	actions    map[string]*actionState
	dialog     *comp.DialogOpts
	dialogOpen bool

	pal           paletteState
	trayChosen    int
	trayHidden    time.Time
	trayIcon      trayIconState
	shortcut      string
	shortcutTaken []string // the shortcuts tried that were taken

	metrics   *Metrics
	quitting  bool
	forceDark *bool // tests run without the app
}

// New makes the app; Start creates its windows once MyGo is ready.
func New(opt Options) *App {
	kit, err := comp.NewKit()
	if err != nil {
		log.Fatal(err)
	}
	a := &App{
		opt:      opt,
		reg:      opt.Registry,
		kit:      kit,
		page:     pageHome,
		actions:  map[string]*actionState{},
		metrics:  newMetrics(opt.StartedAt),
		settings: opt.Defaults,
	}
	a.procs = comp.NewProcessIcons(func() { a.Post(nil) })
	return a
}

// msgs are the catalog messages in the language shown.
func msgs() i18n.Messages { return i18n.In(mygoi18n.Locale()) }

// theme sets the design-system theme for the light or dark appearance.
func (a *App) theme(c *ui.Context) {
	dark := false
	if a.forceDark != nil {
		dark = *a.forceDark
	} else {
		dark = mygo.Theme.IsDark()
	}
	c.SetTheme(ds.MyGoTheme(dark))
}

// Post runs fn on the main thread, then redraws every window.
func (a *App) Post(fn func()) {
	if a.main == nil {
		return
	}
	a.main.Update(func() {
		if fn != nil {
			fn()
		}
		a.redraw()
	})
}

func (a *App) redraw() {
	for _, w := range []*mygo.Window{a.main, a.panel} {
		if w != nil {
			w.Invalidate()
		}
	}
	a.updateTrayIcon()
}

// Start loads the settings, then creates the windows, the tray icon, the
// menu and the global shortcut.
func (a *App) Start() {
	if s, err := a.opt.Store.Load(a.opt.Defaults); err != nil {
		log.Print(err)
	} else {
		a.settings = s
	}
	a.opt.Events.OnChanged(func() { a.Post(nil) })
	a.createMain()
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { a.quitting = true })
	mygo.App.OnWindowAllClosed(func() {}) // the tray keeps the app running
	mygo.App.OnActivate(func(visible bool) {
		if !visible {
			a.openMain("")
		}
	})
	mygo.Theme.OnUpdated(a.redraw)
	mygo.App.OnLocaleChanged(func(string) { a.localeChanged() })
	a.setUpMenu()
	a.setUpTray()
	a.setUpPalette()
	a.registerShortcut()
	go a.tickWhileRunning()
	if a.opt.ScreenshotDir != "" {
		go a.screenshots(a.opt.ScreenshotDir)
	}
}

func (a *App) createMain() {
	a.main = mygo.NewWindow(mygo.WindowOptions{
		Title:         i18n.AppName(),
		Width:         ds.WindowWidth,
		Height:        ds.WindowHeight,
		MinWidth:      ds.WindowMinWidth,
		MinHeight:     ds.WindowMinHeight,
		TitleBarStyle: mygo.TitleBarHidden,
		Vibrancy:      material(a.opt.Material),
		Hidden:        !a.opt.ShowMain,
		StateKey:      "main",
		Content:       ui.View(a.mainView),
	})
	a.main.OnFocus(func() { a.main.Invalidate() })
	a.main.OnBlur(func() { a.main.Invalidate() })
	a.main.OnClose(func(e *mygo.CloseEvent) {
		if !a.quitting {
			e.PreventDefault()
			a.main.Hide()
		}
	})
}

func material(name string) mygo.Vibrancy {
	switch name {
	case "sidebar":
		return mygo.VibrancySidebar
	case "glass":
		return mygo.VibrancyGlass
	case "mica":
		return mygo.VibrancyMica
	}
	return mygo.VibrancyNone
}

// active reports whether the main window is the front window.
func (a *App) active() bool {
	return a.main == nil || a.main.IsFocused()
}

// tickWhileRunning redraws once a second while a switch tool runs, so
// countdowns move in every window.
func (a *App) tickWhileRunning() {
	t := time.NewTicker(tick)
	defer t.Stop()
	for range t.C {
		if len(a.reg.Running()) > 0 {
			a.Post(nil)
		}
	}
}

func (a *App) localeChanged() {
	a.main.SetTitle(i18n.AppName())
	a.setUpMenu()
	a.setTrayMenu()
	a.redraw()
}

// save writes the settings, logging a failure: losing a pin is no reason
// to stop.
func (a *App) save() {
	if err := a.opt.Store.Save(a.settings); err != nil {
		log.Print(err)
	}
}

// openMain shows the main window, at a page when one is given.
func (a *App) openMain(page string) {
	if page != "" {
		a.open(page)
	}
	a.main.Show()
	a.main.Focus()
	mygo.App.Focus()
}

// open shows a page in the main window.
func (a *App) open(page string) {
	a.page, a.pageFilter, a.pal.open = page, "", false
	t := a.reg.Get(page)
	if t == nil {
		return
	}
	sec := string(t.Manifest().Category)
	if a.isPinned(t) {
		sec = sectionPinned
	}
	a.settings.SetCollapsed(sec, false)
	a.settings.Used(page)
	a.save()
}

func (a *App) toggleSidebar() {
	a.sideHidden = !a.sideHidden
	a.setUpMenu()
	a.redraw()
}

// askDialog opens the window-modal confirmation dialog.
func (a *App) askDialog(d comp.DialogOpts) {
	d.Processes = a.procs
	a.dialog, a.dialogOpen = &d, true
}

// activate is a click on a tool in the tray or the palette: a switch tool
// turns on or off, any other tool opens.
func (a *App) activate(t tool.Tool) {
	s, ok := t.(tool.Switch)
	if !ok {
		a.openMain(t.Manifest().ID)
		return
	}
	a.setSwitch(s, !s.Status().On)
}

// runAction runs a tool's quick action; a failure opens the tool's page,
// which shows it.
func (a *App) runAction(t tool.Tool, act *tool.QuickAction) {
	id := t.Manifest().ID
	if act.Run == nil {
		a.openMain(id)
		return
	}
	ok := a.opt.Runner.Do(id, "action", func() {
		if err := act.Run(context.Background()); err != nil {
			log.Printf("%s %s: %v", id, act.ID, err)
		}
	})
	if !ok {
		a.openMain(id)
	}
}
