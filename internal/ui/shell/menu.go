package shell

import (
	"fmt"
	"log"
	"runtime"
	"strings"

	"github.com/egoist/mygo"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// setUpMenu installs the native application menu: oh-my-toys, Edit, View,
// Tools (Mod+1…9), Window.
func (a *App) setUpMenu() {
	item := func(label, acc string, fn func()) *mygo.MenuItem {
		return &mygo.MenuItem{Label: label, Accelerator: acc, Click: func(*mygo.MenuItem, *mygo.Window) { fn() }}
	}
	sideLabel := i18n.HideSidebar()
	if a.sideHidden {
		sideLabel = i18n.ShowSidebar()
	}
	mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Role: mygo.RoleEditMenu},
		{Label: i18n.MenuView(), Submenu: []*mygo.MenuItem{
			item(sideLabel, "CmdOrCtrl+Shift+S", a.toggleSidebar),
			mygo.Separator(),
			{Label: i18n.Appearance(), Submenu: []*mygo.MenuItem{
				item(i18n.ThemeAuto(), "", func() { mygo.Theme.SetSource(mygo.ThemeSystem) }),
				item(i18n.ThemeDark(), "", func() { mygo.Theme.SetSource(mygo.ThemeDark) }),
				item(i18n.ThemeLight(), "", func() { mygo.Theme.SetSource(mygo.ThemeLight) }),
			}},
		}},
		{Label: i18n.MenuTools(), Submenu: a.toolsMenu(item)},
		{Role: mygo.RoleWindowMenu},
	}))
}

func (a *App) toolsMenu(item func(string, string, func()) *mygo.MenuItem) []*mygo.MenuItem {
	items := []*mygo.MenuItem{
		item(i18n.PaletteTitle(), "CmdOrCtrl+K", a.showPalette),
	}
	if a.opt.Snapshots != nil {
		items = append(items, item(i18n.SnapMenu(), "CmdOrCtrl+Shift+A", a.startSnap))
	}
	items = append(items,
		item(i18n.TrayTitle(), "", a.toggleTray),
		item(i18n.AllTools(), "", func() { a.openMain(pageHome) }),
		item(i18n.SettingsEllipsis(), "CmdOrCtrl+,", func() { a.openMain(pageSettings) }),
	)
	all := a.reg.All()
	if len(all) == 0 {
		return items
	}
	items = append(items, mygo.Separator())
	for i, t := range all[:min(len(all), ds.SidebarMaxJump)] {
		id, name := t.Manifest().ID, t.Manifest().Name(msgs())
		items = append(items, item(name, fmt.Sprintf("CmdOrCtrl+%d", i+1), func() { a.openMain(id) }))
	}
	return items
}

// GOOS names of the platforms that own different OS pieces.
const (
	goosMac   = "darwin"
	goosLinux = "linux"
)

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

// keys spells an accelerator in the user's key names: ⌘K on macOS, Ctrl+K
// elsewhere (Kbd spec: the only text that differs between platforms).
func keys(acc string) string {
	if runtime.GOOS != goosMac {
		return strings.ReplaceAll(acc, "CmdOrCtrl", "Ctrl")
	}
	return strings.NewReplacer("CmdOrCtrl+", "⌘", "Cmd+", "⌘", "Ctrl+", "⌃", "Alt+", "⌥", "Shift+", "⇧").Replace(acc)
}

func modKey(key string) string { return keys("CmdOrCtrl+" + key) }
