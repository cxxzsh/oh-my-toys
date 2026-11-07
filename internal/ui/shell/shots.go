package shell

import (
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
)

// Pauses of the screenshot run: for the app to settle, and for a frame.
const (
	shotsSettle = 1500 * time.Millisecond
	shotsFrame  = 700 * time.Millisecond
)

// ShowMain shows the main window as it was.
func (a *App) ShowMain() { a.openMain("") }

// screenshots goes through the main screens in each language and theme,
// capturing the native UI of each window with Window.CapturePage, then
// quits.
func (a *App) screenshots(dir string) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		log.Print(err)
		return
	}
	time.Sleep(shotsSettle)
	for _, lang := range mygo.App.Languages() {
		mygo.App.SetLocale(lang)
		for _, theme := range []mygo.ThemeSource{mygo.ThemeLight, mygo.ThemeDark} {
			mygo.Theme.SetSource(theme)
			a.shootScreens(dir, lang+"-"+string(theme))
		}
	}
	mygo.App.SetLocale(a.settings.Language)
	mygo.Theme.SetSource(mygo.ThemeSystem)
	a.Post(func() { a.quitting = true; mygo.App.Quit() })
}

func (a *App) shootScreens(dir, suffix string) {
	a.Post(func() { a.openMain(pageHome); a.homeCat = "" })
	a.capture(a.main, dir, "home-"+suffix)
	for _, t := range a.pinnedTools() {
		id := t.Manifest().ID
		a.Post(func() { a.open(id) })
		a.capture(a.main, dir, id+"-"+suffix)
	}
	a.Post(func() { a.open(pageSettings) })
	a.capture(a.main, dir, "settings-"+suffix)
	a.Post(func() { a.open(pageHome); a.showPalette() })
	a.capture(a.main, dir, "palette-"+suffix)
	a.Post(func() { a.pal.open = false; a.toggleTray() })
	a.capture(a.panel, dir, "tray-"+suffix)
	a.Post(a.hideTray)
}

func (a *App) capture(w *mygo.Window, dir, name string) {
	time.Sleep(shotsFrame)
	if w == nil {
		return
	}
	png, err := w.CapturePage()
	if err != nil {
		log.Printf("capture %s: %v", name, err)
		return
	}
	path := filepath.Join(dir, name+".png")
	if err := os.WriteFile(path, png, 0o600); err != nil {
		log.Print(err)
	}
}
