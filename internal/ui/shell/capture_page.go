package shell

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// capturePage is the page of a tool the app runs itself, such as the
// screenshot: the button that starts it, and the settings that belong to it.
func (a *App) capturePage(c *ui.Context, t tool.Tool) {
	p := ds.P(c)
	m := t.Manifest()
	ui.Column(c).Grow(1).FillWidth().Padding(ds.Space16).Gap(ds.Space16).Children(func() {
		comp.PageHeader(c, comp.Header{Title: m.Name(msgs()), Description: m.Summary(msgs())})
		comp.Card(c).FillWidth().Padding(ds.Space16).Gap(ds.Space12).Children(func() {
			ui.Row(c).Gap(ds.Space12).AlignItems(ui.Center).Children(func() {
				if a.kit.Button(c.Key("capture-run"), i18n.SnapMenu(), comp.ButtonOpts{Kind: comp.Primary}).Clicked() {
					a.startSnap()
				}
				if a.opt.SnapShortcut != "" {
					comp.KbdLarge(c, keys(a.opt.SnapShortcut))
				}
			})
			comp.Text(c, i18n.SnapHint(), ds.Body, p.TextSecondary)
		})
		comp.Card(c).FillWidth().Padding(ds.Space8, ds.Space16).Children(func() {
			a.screenshotSaveRow(c)
		})
	})
}

// screenshotSaveRow is where screenshots are saved: the folder, and the way
// to change it.
func (a *App) screenshotSaveRow(c *ui.Context) {
	p := ds.P(c)
	ui.Row(c).FillWidth().Height(ds.TableRow).Gap(ds.Space12).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Children(func() {
			comp.Text(c, i18n.SnapSaveTo(), ds.Body, p.TextPrimary)
			comp.Text(c, snapPathLabel(a.snapDir(), homeDir()), ds.Caption, p.TextInactive).SingleLine().Ellipsis("…")
		})
		if a.settings.ScreenshotDir != "" {
			if a.kit.Button(c.Key("snap-dir-default"), i18n.SnapSaveDefault(), comp.ButtonOpts{Kind: comp.Ghost, Small: true}).Clicked() {
				a.setScreenshotDir("")
			}
		}
		if a.kit.Button(c.Key("snap-dir-change"), i18n.SnapSaveChange(), comp.ButtonOpts{Kind: comp.Secondary, Small: true}).Clicked() {
			a.pickScreenshotDir()
		}
	})
}

func (a *App) pickScreenshotDir() {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent:            a.main,
		Title:             i18n.SnapSaveTo(),
		DefaultPath:       a.snapDir(),
		Directory:         true,
		CreateDirectories: true,
		ButtonLabel:       i18n.SnapSaveChange(),
	})
	if err != nil || len(paths) == 0 {
		return
	}
	a.setScreenshotDir(paths[0])
}

func (a *App) setScreenshotDir(dir string) {
	a.settings.ScreenshotDir = dir
	a.save()
	a.redraw()
}
