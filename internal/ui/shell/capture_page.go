package shell

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

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
		if a.opt.ScreenAllowed != nil && !a.opt.ScreenAllowed() {
			a.permissionBanner(c)
		}
		comp.Card(c).FillWidth().Padding(ds.Space16).Gap(ds.Space12).Children(func() {
			ui.Row(c).FillWidth().Gap(ds.Space16).AlignItems(ui.Center).Children(func() {
				a.kit.ToolIcon(c, m, comp.ToolIconOpts{Size: comp.IconLarge, Tint: true})
				ui.Column(c).Grow(1).Gap(ds.Space6).Children(func() {
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
			})
		})
		comp.Card(c).FillWidth().Padding(ds.Space8, ds.Space16).Children(func() {
			a.screenshotSaveRow(c)
		})
		a.recentCapturesCard(c)
	})
}

// permissionBanner says the screen-recording permission is missing, which
// stops captures, with the glyph and tone of a warning.
func (a *App) permissionBanner(c *ui.Context) {
	p := ds.P(c)
	ui.Row(c).FillWidth().Padding(ds.Space12, ds.Space16).Gap(ds.Space12).AlignItems(ui.Center).
		Radius(ds.RadiusSm).Background(p.WarningSoft).Border(ds.LineWidth, p.Warning).Children(func() {
		a.kit.Glyph(c, "lock", ds.GlyphSm, p.Warning)
		comp.Text(c, i18n.SnapPermRequired(), ds.Body, p.TextPrimary).Grow(1)
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

// recentCapturesCard lists the newest screenshots in the save folder, each
// name copied in full on click, with a button to open the folder itself.
func (a *App) recentCapturesCard(c *ui.Context) {
	p := ds.P(c)
	dir := a.snapDir()
	a.ensureSnapRecents(dir)
	comp.Card(c).FillWidth().Padding(ds.Space16).Gap(ds.Space12).Children(func() {
		ui.Row(c).FillWidth().Gap(ds.Space12).AlignItems(ui.Center).Children(func() {
			comp.Text(c, i18n.SnapRecentTitle(), ds.Label, p.TextStrong).Grow(1)
			if a.kit.Button(c.Key("snap-open-folder"), i18n.SnapOpenFolder(), comp.ButtonOpts{Kind: comp.Secondary, Small: true}).Clicked() {
				_ = mygo.Shell.OpenPath(dir)
			}
		})
		if len(a.snapRecents) == 0 {
			comp.Text(c, i18n.SnapRecentEmpty(), ds.Body, p.TextInactive)
			return
		}
		for _, f := range a.snapRecents {
			a.recentRow(c, dir, f)
		}
	})
}

func (a *App) ensureSnapRecents(dir string) {
	if a.snapRecentsDir == dir || a.opt.Runner == nil {
		return
	}
	a.snapRecentsDir = dir
	a.opt.Runner.Go(context.Background(), "screenshot", 5*time.Second, func(context.Context) {
		files := recentPNGs(dir, 5)
		a.Post(func() { a.snapRecents = files })
	})
}

// recentRow is one capture: its mono name and size, and a click copies its
// full path.
func (a *App) recentRow(c *ui.Context, dir string, f recentFile) {
	p := ds.P(c)
	b := ui.ButtonBase(c.Key("recent:" + f.name)).Cursor(ui.CursorDefault)
	if b.Clicked() {
		mygo.Clipboard.WriteText(filepath.Join(dir, f.name))
		c.Toast(i18n.Copied(f.name))
	}
	mono := ds.Caption
	mono.Mono = true
	b.FillWidth().MinHeight(ds.TableRow).Gap(ds.Space12).AlignItems(ui.Center).Children(func() {
		comp.Text(c, f.name, mono, p.TextPrimary).Grow(1).SingleLine().Ellipsis("…")
		comp.Value(c, humanSize(f.size), p.TextSecondary)
	})
}

// recentFile is one screenshot in the save folder.
type recentFile struct {
	name    string
	size    int64
	modTime time.Time
}

// recentPNGs lists up to n .png files of dir, newest first. It reads only the
// one flat folder and skips anything that is not a readable file.
func recentPNGs(dir string, n int) []recentFile {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var files []recentFile
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".png") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, recentFile{name: e.Name(), size: info.Size(), modTime: info.ModTime()})
	}
	slices.SortFunc(files, func(a, b recentFile) int { return b.modTime.Compare(a.modTime) })
	return files[:min(len(files), n)]
}

// humanSize writes a byte count as KB or MB.
func humanSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	default:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
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
	a.snapRecentsDir = ""
	a.save()
	a.redraw()
}
