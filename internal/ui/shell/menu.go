package shell

import (
	"log"
	"runtime"
	"strings"

	"github.com/egoist/mygo"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
)

// setUpMenu installs the application menu: the system's app, edit and
// window menus (the edit menu gives text fields copy and paste on macOS),
// and a menu that opens the palette, the tray panel and the tools.
func (a *App) setUpMenu() {
	item := func(label string, fn func()) *mygo.MenuItem {
		return &mygo.MenuItem{Label: label, Click: func(*mygo.MenuItem, *mygo.Window) { fn() }}
	}
	mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Role: mygo.RoleEditMenu},
		{Label: i18n.MenuTools(), Submenu: []*mygo.MenuItem{
			item(i18n.PaletteTitle(), a.showPalette),
			item(i18n.TrayTitle(), a.toggleTray),
			item(i18n.AllTools(), func() { a.openMain(pageHome) }),
			item(i18n.Settings(), func() { a.openMain(pageSettings) }),
		}},
		{Role: mygo.RoleWindowMenu},
	}))
}

// GOOS names of the platforms that behave apart.
const (
	goosMac   = "darwin"
	goosLinux = "linux"
)

// registerShortcut takes the first free global shortcut of the options,
// and remembers those taken, which the settings page names.
func (a *App) registerShortcut() {
	a.shortcutTaken = nil
	for _, s := range a.opt.Shortcuts {
		if err := mygo.GlobalShortcut.Register(s, a.togglePalette); err != nil {
			log.Printf("shortcut %s: %v", s, err)
			a.shortcutTaken = append(a.shortcutTaken, s)
			continue
		}
		a.shortcut = s
		return
	}
}

// keys spells an accelerator the platform's way: ⌥Space on macOS,
// Alt+Space elsewhere.
func keys(acc string) string {
	if runtime.GOOS != goosMac {
		return strings.ReplaceAll(acc, "CmdOrCtrl", "Ctrl")
	}
	return strings.NewReplacer("CmdOrCtrl+", "⌘", "Cmd+", "⌘", "Ctrl+", "⌃", "Alt+", "⌥", "Shift+", "⇧").Replace(acc)
}

// modKey is the hint of a ⌘ / Ctrl shortcut with a key.
func modKey(key string) string { return keys("CmdOrCtrl+" + key) }
