package shell

import (
	"strings"

	"github.com/egoist/mygo"
	mygoi18n "github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// devVersion is the version of a build that mygo build did not stamp.
const devVersion = "dev"

// msgsLang is the tag of the language shown.
func msgsLang() string { return mygoi18n.Locale() }

func (a *App) settingsPage(c *ui.Context) {
	p := ds.P(c)
	comp.Text(c, i18n.Settings(), ds.LargeTitle, p.TextPrimary).Role(ui.RoleHeading).Level(1)
	a.section(c, i18n.Appearance(), func() {
		a.choiceRow(c, i18n.Language(), a.languageChoices())
		a.choiceRow(c, i18n.Theme(), a.themeChoices())
	})
	a.section(c, i18n.PaletteTitle(), func() { a.shortcutRow(c) })
	a.section(c, i18n.About(), func() {
		version := mygo.App.Version()
		if version == "" {
			version = devVersion
		}
		if a.opt.Portable {
			version = i18n.VersionPortable(version)
		}
		a.valueRow(c, i18n.Version(), version)
	})
	if a.opt.Dev {
		a.section(c, i18n.Measurements(), func() { a.measurements(c) })
	}
}

func (a *App) section(c *ui.Context, title string, rows func()) {
	ui.Column(c).FillWidth().Gap(ds.Space2).Children(func() {
		comp.Eyebrow(c, title)
		comp.Card(c).FillWidth().Padding(ds.Space2, ds.Space4).Children(rows)
	})
}

func (a *App) valueRow(c *ui.Context, name, value string) {
	p := ds.P(c)
	ui.Row(c.Key(name)).FillWidth().MinHeight(ds.RowHeight).Gap(ds.Space3).AlignItems(ui.Center).Children(func() {
		comp.Text(c, name, ds.Headline, p.TextPrimary).Grow(1)
		comp.Text(c, value, ds.Body, p.TextSecondary).FontFeatures("tnum").Selectable()
	})
}

// choice is one button of a row of choices.
type choice struct {
	label  string
	chosen bool
	choose func()
}

func (a *App) choiceRow(c *ui.Context, name string, choices []choice) {
	p := ds.P(c)
	ui.Row(c.Key(name)).FillWidth().MinHeight(ds.RowHeight).Gap(ds.Space2).AlignItems(ui.Center).Children(func() {
		comp.Text(c, name, ds.Headline, p.TextPrimary).Grow(1)
		for _, ch := range choices {
			kind := comp.Secondary
			if ch.chosen {
				kind = comp.Primary
			}
			if comp.Button(c.Key(ch.label), ch.label, kind).Clicked() {
				ch.choose()
			}
		}
	})
}

// languageChoices are the system's language and each catalog's language,
// named in itself.
func (a *App) languageChoices() []choice {
	saved, langs := a.settings.Language, mygo.App.Languages()
	out := make([]choice, 0, 1+len(langs))
	out = append(out, choice{label: i18n.FollowSystem(), chosen: saved == "", choose: func() { a.setLanguage("") }})
	for _, tag := range langs {
		out = append(out, choice{
			label:  mygoi18n.Autonym(tag),
			chosen: saved == tag,
			choose: func() { a.setLanguage(tag) },
		})
	}
	return out
}

// setLanguage shows a language, or the system's with "", and remembers the
// choice: MyGo keeps the language itself, but does not say which was chosen.
func (a *App) setLanguage(tag string) {
	a.settings.Language = tag
	a.save()
	mygo.App.SetLocale(tag)
}

func (a *App) themeChoices() []choice {
	src := mygo.Theme.Source()
	set := func(s mygo.ThemeSource) func() { return func() { mygo.Theme.SetSource(s) } }
	return []choice{
		{label: i18n.FollowSystem(), chosen: src == mygo.ThemeSystem || src == "", choose: set(mygo.ThemeSystem)},
		{label: i18n.ThemeLight(), chosen: src == mygo.ThemeLight, choose: set(mygo.ThemeLight)},
		{label: i18n.ThemeDark(), chosen: src == mygo.ThemeDark, choose: set(mygo.ThemeDark)},
	}
}

func (a *App) shortcutRow(c *ui.Context) {
	p := ds.P(c)
	value := i18n.NoShortcut()
	if a.shortcut != "" {
		value = keys(a.shortcut)
	}
	a.valueRow(c, i18n.GlobalShortcut(), value)
	if len(a.shortcutTaken) == 0 {
		return
	}
	taken := make([]string, 0, len(a.shortcutTaken))
	for _, s := range a.shortcutTaken {
		taken = append(taken, keys(s))
	}
	ui.Row(c).FillWidth().Gap(ds.Space2).Padding(0, 0, ds.Space3).Children(func() {
		comp.Glyph(c, "triangle-alert", ds.Glyph, p.Warning)
		comp.Text(c, i18n.ShortcutTaken(strings.Join(taken, i18n.ListSeparator())), ds.Callout, p.Warning).Grow(1)
	})
}
