package shell

import (
	"strings"

	"github.com/egoist/mygo"
	mygoi18n "github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

const mcpSnippet = `{"mcpServers":{"oh-my-toys":{"command":"omt","args":["mcp"]}}}`

// devVersion is the version of a build that mygo build did not stamp.
const devVersion = "dev"

// msgsLang is the tag of the language shown.
func msgsLang() string { return mygoi18n.Locale() }

func (a *App) settingsPage(c *ui.Context) {
	comp.PageHeader(c, comp.Header{Title: i18n.Settings(), Description: i18n.SettingsDesc()})
	a.kit.SettingsList(c, []comp.SettingsRow{
		{
			Glyph:   "contrast",
			Title:   i18n.Appearance(),
			Desc:    i18n.AppearanceDesc(),
			Control: func() { a.themeSegment(c) },
		},
		{
			Glyph:   "language",
			Title:   i18n.Language(),
			Desc:    i18n.LanguageDesc(),
			Control: func() { a.languageSelect(c) },
		},
		{
			Glyph:   "keyboard",
			Title:   i18n.GlobalShortcut(),
			Desc:    a.shortcutDesc(),
			Control: func() { a.shortcutBadge(c) },
		},
		{
			Glyph: "tune",
			Title: i18n.ResetSidebar(),
			Desc:  i18n.ResetSidebarDesc(),
			Control: func() {
				if a.kit.Button(c.Key("reset-side"), i18n.ResetSidebarAction(), comp.ButtonOpts{Kind: comp.Secondary}).Clicked() {
					a.resetSidebar()
				}
			},
		},
	})
	a.agentsSection(c)
	a.aboutSection(c)
	if a.opt.Dev {
		a.section(c, i18n.Measurements(), func() { a.measurements(c) })
	}
}

// resetSidebar puts the sidebar back to the first-launch order, pins and
// open sections.
func (a *App) resetSidebar() {
	a.settings.Reset(a.opt.Defaults)
	a.save()
}

func (a *App) languageSelect(c *ui.Context) {
	saved, langs := a.settings.Language, mygo.App.Languages()
	tags := make([]string, 0, 1+len(langs))
	names := make([]string, 0, 1+len(langs))
	tags = append(tags, "")
	names = append(names, i18n.FollowSystem())
	sel := 0
	for i, tag := range langs {
		tags = append(tags, tag)
		names = append(names, mygoi18n.Autonym(tag))
		if tag == saved {
			sel = i + 1
		}
	}
	a.kit.Select(c, names, sel, func(i int) { a.setLanguage(tags[i]) })
}

func (a *App) setLanguage(tag string) {
	a.settings.Language = tag
	a.save()
	mygo.App.SetLocale(tag)
}

func (a *App) shortcutBadge(c *ui.Context) {
	if a.shortcut == "" {
		comp.Badge(c, i18n.NoShortcut(), comp.ToneWarning)
		return
	}
	comp.Kbd(c, keys(a.shortcut))
}

func (a *App) shortcutTakenDesc() string {
	if len(a.shortcutTaken) == 0 {
		return ""
	}
	taken := make([]string, 0, len(a.shortcutTaken))
	for _, s := range a.shortcutTaken {
		taken = append(taken, keys(s))
	}
	return i18n.ShortcutTaken(strings.Join(taken, i18n.ListSeparator()))
}

func (a *App) shortcutDesc() string {
	if d := a.shortcutTakenDesc(); d != "" {
		return d
	}
	return i18n.SettingsShortcutDesc()
}

func (a *App) aboutSection(c *ui.Context) {
	version := mygo.App.Version()
	if version == "" {
		version = devVersion
	}
	if a.opt.Portable {
		version = i18n.VersionPortable(version)
	}
	ui.Column(c).FillWidth().Gap(ds.Space8).Children(func() {
		comp.Eyebrow(c, i18n.About())
		a.kit.SettingsList(c, []comp.SettingsRow{
			{
				Glyph:   "memory",
				Title:   i18n.Version(),
				Control: func() { comp.BadgeRimmed(c, version) },
			},
		})
	})
}

func (a *App) agentsSection(c *ui.Context) {
	ui.Column(c).FillWidth().Gap(ds.Space8).Children(func() {
		comp.Eyebrow(c, i18n.AgentsSection())
		a.kit.SettingsList(c, []comp.SettingsRow{
			{
				Glyph: "smart_toy",
				Title: i18n.AgentsMcpTitle(),
				Desc:  i18n.AgentsMcpDesc(),
				Control: func() {
					if a.kit.Button(c.Key("copy-mcp"), i18n.CopyConfig(), comp.ButtonOpts{Kind: comp.Secondary, Small: true, Glyph: "content_copy"}).Clicked() {
						mygo.Clipboard.WriteText(mcpSnippet)
						c.Toast(i18n.Copied("omt mcp"))
					}
				},
			},
			{
				Glyph:   "terminal",
				Title:   i18n.AgentsCliTitle(),
				Desc:    i18n.AgentsCliDesc(),
				Control: func() { a.kit.CLICopyPill(c.Key("cli-help"), "omt --help", "omt --help", i18n.Copied("omt --help")) },
			},
			{
				Glyph: "lock",
				Title: i18n.AgentsRiskTitle(),
				Desc:  i18n.AgentsRiskDesc(),
				Control: func() {
					ui.Row(c).Gap(ds.Space4).AlignItems(ui.Center).Children(func() {
						comp.RiskBadge(c, tool.RiskRead)
						comp.RiskBadge(c, tool.RiskSystem)
						comp.RiskBadge(c, tool.RiskWrite)
						comp.RiskBadge(c, tool.RiskProcess)
					})
				},
			},
		})
	})
}

func (a *App) section(c *ui.Context, title string, rows func()) {
	ui.Column(c).FillWidth().Gap(ds.Space8).Children(func() {
		comp.Eyebrow(c, title)
		comp.Card(c).FillWidth().Padding(ds.Space8, ds.Space16).Children(rows)
	})
}

func (a *App) valueRow(c *ui.Context, name, value string) {
	p := ds.P(c)
	ui.Row(c.Key(name)).FillWidth().MinHeight(ds.TableRow).Gap(ds.Space12).AlignItems(ui.Center).Children(func() {
		comp.Text(c, name, ds.Body, p.TextPrimary).Grow(1)
		comp.Value(c, value, p.TextSecondary).Selectable()
	})
}
