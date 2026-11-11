package shell

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"

	xdraw "golang.org/x/image/draw"
)

// SnapShot is one display's picture for the screenshot session, in
// device pixels, with the display's rectangle in global screen points.
// The entry point fills it from the platform's capture.
type SnapShot struct {
	ID     uint32
	Bounds image.Rectangle
	Image  *image.RGBA
}

// snapOverlay is the borderless window over one display.
type snapOverlay struct {
	shot SnapShot
	win  *mygo.Window
	bmp  *ui.Bitmap
}

// pixelsPerPoint is the display's scale: image pixels over points.
func (o *snapOverlay) pixelsPerPoint() float64 {
	b := o.shot.Bounds
	w := o.shot.Image.Bounds().Dx()
	if b.Dx() <= 0 || w <= 0 {
		return 1
	}
	return float64(w) / float64(b.Dx())
}

// snapSession is one run of the screenshot tool: the pictures taken, one
// window per display, and the selection in global points.
type snapSession struct {
	overlays []*snapOverlay

	sel      image.Rectangle // global points; empty when nothing is selected
	anchor   image.Point     // where the drag in progress began
	origin   image.Rectangle // the selection when a move or resize began
	hover    image.Point
	hoverOn  bool
	mode     snapGrip
	dragging bool
	pressed  []bool // per overlay: its surface is being pressed
	wasDown  bool
	bar      image.Rectangle // toolbar, in global points; empty when hidden

	tool     snapTool          // the annotation tool in hand; none moves the selection
	marks    []snapMark        // annotations, in global points
	history  []snapAction      // changes to the marks, oldest first
	redone   []snapAction      // changes undo took back
	pending  *snapMark         // the one being dragged
	text     *snapText         // the label being typed
	wins     []image.Rectangle // window frames a click can take as the selection
	pressWin image.Rectangle   // the window under the press that began a drag

	flash     string
	flashTil  time.Time
	returnPID int // the app that was frontmost, to give the keyboard back
}

// snapText is a label being typed.
type snapText struct {
	at     image.Point
	value  string
	commit bool // Return was taken: the label is kept next frame, once an input method's text has landed
}

// startSnap is the screenshot hotkey: take the pictures and cover every
// display with an overlay.
func (a *App) startSnap() {
	if a.snap != nil || a.snapping || a.opt.Snapshots == nil {
		return
	}
	if a.opt.ScreenAllowed != nil && !a.opt.ScreenAllowed() {
		a.askScreenPermission()
		return
	}
	// Still inside the hot-key event, so macOS allows the activation.
	// Remember who had the keyboard before we take it.
	a.snapPrev = 0
	if a.opt.Frontmost != nil && a.opt.IsFrontmost != nil && !a.opt.IsFrontmost() {
		a.snapPrev = a.opt.Frontmost()
	}
	if a.opt.ActivateSelf != nil {
		a.opt.ActivateSelf()
	}
	a.snapping = true
	shots, err := a.opt.Snapshots()
	if err != nil {
		a.snapping = false
		a.giveBackKeys()
		log.Printf("screenshot: %v", err)
		return
	}
	a.Post(func() { a.showSnap(shots) })
}

// registerSnapShortcut binds the screenshot hotkey when the tool is on.
func (a *App) registerSnapShortcut() {
	if a.opt.SnapShortcut == "" || a.opt.Snapshots == nil {
		return
	}
	if err := mygo.GlobalShortcut.Register(a.opt.SnapShortcut, func() { a.Post(a.startSnap) }); err != nil {
		log.Printf("screenshot shortcut %s: %v", a.opt.SnapShortcut, err)
	}
}

// snapKeyBinds are the keys that work while the overlay shows, even
// when another app is frontmost: Carbon hot keys, not the window.
func snapKeyBinds() []struct {
	acc    string
	fn     func(*App)
	typing bool // unbound while a label is typed: the field takes Return
} {
	return []struct {
		acc    string
		fn     func(*App)
		typing bool
	}{
		{"Escape", (*App).snapCancel, false},
		{"Return", (*App).snapReturn, true},
		{"CmdOrCtrl+S", (*App).snapSave, false},
		{"CmdOrCtrl+Z", (*App).snapUndo, true},
		{"CmdOrCtrl+Shift+Z", (*App).snapRedo, true},
	}
}

func (a *App) registerSnapKeys() {
	a.unregisterSnapKeys()
	if a.main == nil {
		return
	}
	typing := a.snap != nil && a.snap.text != nil
	for _, b := range snapKeyBinds() {
		if b.typing && typing {
			continue
		}
		fn := b.fn
		if err := mygo.GlobalShortcut.Register(b.acc, func() { fn(a) }); err != nil {
			log.Printf("screenshot key %s: %v", b.acc, err)
			continue
		}
		a.snapKeys = append(a.snapKeys, b.acc)
	}
}

func (a *App) unregisterSnapKeys() {
	for _, acc := range a.snapKeys {
		mygo.GlobalShortcut.Unregister(acc)
	}
	a.snapKeys = nil
}

// askScreenPermission offers to open the privacy settings when the app
// may not take screenshots.
func (a *App) askScreenPermission() {
	a.askDialog(comp.DialogOpts{
		Title:       i18n.SnapPermTitle(),
		Description: i18n.SnapPermBody(),
		Cancel:      i18n.Cancel(),
		Confirm:     i18n.SnapPermOpen(),
		OnConfirm: func() {
			_ = mygo.Shell.OpenExternal("x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture")
		},
	})
}

// showSnap opens the overlay of every shot, on the main thread.
func (a *App) showSnap(shots []SnapShot) {
	a.snapping = false
	if len(shots) == 0 || a.snap != nil {
		a.snapPrev = 0
		return
	}
	s := &snapSession{pressed: make([]bool, len(shots)), returnPID: a.snapPrev}
	a.snapPrev = 0
	if a.opt.Windows != nil {
		s.wins = a.opt.Windows()
	}
	a.snap = s
	for _, shot := range shots {
		ov := &snapOverlay{shot: shot, bmp: ui.NewBitmap(shot.Image)}
		s.overlays = append(s.overlays, ov)
		a.newSnapWindow(ov)
	}
	// Keys are Carbon hot keys, so another app can stay frontmost.
	// Focusing the panel would swallow Escape before the hot key.
	a.registerSnapKeys()
}

// newSnapWindow creates the borderless overlay of one display.
func (a *App) newSnapWindow(ov *snapOverlay) {
	b := ov.shot.Bounds
	ov.win = mygo.NewWindow(mygo.WindowOptions{
		Width:             b.Dx(),
		Height:            b.Dy(),
		Frameless:         true,
		Transparent:       true,
		AlwaysOnTop:       true,
		SkipTaskbar:       true,
		DisableResize:     true,
		DisableMove:       true,
		DisableMinimize:   true,
		DisableMaximize:   true,
		DisableClose:      true,
		DisableFullScreen: true,
		DisableShadow:     true,
		Panel:             true,
		Content:           ui.View(func(c *ui.Context) { a.snapView(c, ov) }),
	})
	ov.win.Show()
	if a.opt.OverlayWindow != nil {
		a.opt.OverlayWindow(ov.win.NativeHandle())
	}
	ov.win.SetBounds(mygo.Rectangle{X: b.Min.X, Y: b.Min.Y, Width: b.Dx(), Height: b.Dy()})
}

// snapCancel steps back from what is in hand: a label being typed, the
// annotation tool, or the selection, and closes the overlay when nothing
// is left to step back from.
func (a *App) snapCancel() {
	s := a.snap
	if s == nil {
		return
	}
	switch {
	case s.text != nil:
		s.text = nil
		a.registerSnapKeys()
	case s.pending != nil:
		s.pending, s.dragging, s.mode = nil, false, snapGripNone
	case s.tool != snapToolNone:
		s.tool = snapToolNone
	default:
		a.endSnap()
		return
	}
	a.snapInvalidate()
}

// snapUndo takes back the annotation being drawn, or the last one kept.
func (a *App) snapUndo() {
	s := a.snap
	if s == nil || !s.undo() {
		return
	}
	a.snapInvalidate()
}

// snapRedo puts back the annotation undo took back last.
func (a *App) snapRedo() {
	s := a.snap
	if s == nil || !s.redo() {
		return
	}
	a.snapInvalidate()
}

// snapReturn commits the label being typed, if any, or copies the
// selection.
func (a *App) snapReturn() {
	s := a.snap
	if s == nil {
		return
	}
	if s.text != nil {
		a.snapCommitText()
		return
	}
	a.snapCopy()
}

// snapCommitText adds the label being typed to the marks.
func (a *App) snapCommitText() {
	s := a.snap
	if s == nil || s.text == nil {
		return
	}
	t := s.text
	s.text = nil
	a.registerSnapKeys()
	if strings.TrimSpace(t.value) != "" {
		s.marks = append(s.marks, snapMark{tool: snapToolText, a: t.at, text: t.value})
	}
	a.snapInvalidate()
}

// endSnap closes every overlay and forgets the session.
func (a *App) endSnap() {
	s := a.snap
	a.snap = nil
	a.unregisterSnapKeys()
	if s == nil {
		return
	}
	for _, ov := range s.overlays {
		ov.bmp.ReleasePixels()
		ov.win.Close()
	}
	a.snapPrev = s.returnPID
	a.giveBackKeys()
}

// giveBackKeys returns the keyboard to the app that had it, if this
// app took it and still has it.
func (a *App) giveBackKeys() {
	pid := a.snapPrev
	a.snapPrev = 0
	if pid > 0 && a.opt.IsFrontmost != nil && a.opt.IsFrontmost() && a.opt.Activate != nil {
		a.opt.Activate(pid)
	}
}

// snapInvalidate redraws every overlay after the session changed.
func (a *App) snapInvalidate() {
	if s := a.snap; s != nil {
		for _, ov := range s.overlays {
			ov.win.Invalidate()
		}
	}
}

// snapFlash shows a transient message in the overlays' hint line.
func (a *App) snapFlash(text string) {
	if s := a.snap; s != nil {
		s.flash, s.flashTil = text, time.Now().Add(snapFlashFor)
		a.snapInvalidate()
	}
}

// snapCopy puts the selection on the clipboard as a PNG. The encode
// runs off the main thread; the clipboard write returns to it.
func (a *App) snapCopy() {
	if s := a.snap; s != nil && s.text != nil {
		a.snapCommitText()
	}
	img, err := a.snapImage()
	if err != nil {
		a.snapFlash(err.Error())
		return
	}
	go func() {
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			a.Post(func() { a.snapFlash(err.Error()) })
			return
		}
		a.Post(func() {
			if err := mygo.Clipboard.WriteImage(buf.Bytes()); err != nil {
				a.snapFlash(err.Error())
				return
			}
			a.endSnap()
		})
	}()
}

// snapSave writes the selection as a PNG file and flashes its path.
// Like snapCopy, the encode runs off the main thread.
func (a *App) snapSave() {
	if s := a.snap; s != nil && s.text != nil {
		a.snapCommitText()
	}
	img, err := a.snapImage()
	if err != nil {
		a.snapFlash(err.Error())
		return
	}
	dir, fellBack, err := a.snapSaveDir()
	if err != nil {
		a.snapFlash(err.Error())
		return
	}
	path := filepath.Join(dir, snapFileName(dir, time.Now()))
	go func() {
		f, err := os.Create(path) //nolint:gosec // G304: the name is a timestamp we generated in dir
		if err != nil {
			a.Post(func() { a.snapFlash(err.Error()) })
			return
		}
		defer func() { _ = f.Close() }()
		if err := png.Encode(f, img); err != nil {
			a.Post(func() { a.snapFlash(err.Error()) })
			return
		}
		a.Post(func() {
			if fellBack {
				a.snapFlash(i18n.SnapSaveFellBack(path))
				return
			}
			a.snapFlash(i18n.SnapSaved(path))
		})
	}()
}

// snapDir is where the next screenshot is saved: an override, the
// chosen folder, or ~/Pictures/oh-my-toys.
func (a *App) snapDir() string {
	if a.opt.SaveDir != "" {
		return a.opt.SaveDir
	}
	if dir := strings.TrimSpace(a.settings.ScreenshotDir); dir != "" {
		return dir
	}
	dir, err := snapDefaultDir()
	if err != nil {
		return ""
	}
	return dir
}

// snapSaveDir creates the save folder. When the chosen one cannot be
// made, it uses the default and reports that it fell back.
func (a *App) snapSaveDir() (dir string, fellBack bool, err error) {
	dir = a.snapDir()
	if dir == "" {
		return "", false, fmt.Errorf("no save folder")
	}
	if os.MkdirAll(dir, 0o700) == nil {
		return dir, false, nil
	}
	def, derr := snapDefaultDir()
	if derr != nil || def == dir || os.MkdirAll(def, 0o700) != nil {
		return "", false, fmt.Errorf("save folder: %s", dir)
	}
	return def, true, nil
}

func snapDefaultDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Pictures", "oh-my-toys"), nil
}

func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

// snapPathLabel shortens a path under home to a ~/… form.
func snapPathLabel(dir, home string) string {
	if home != "" && (dir == home || strings.HasPrefix(dir, home+string(os.PathSeparator))) {
		return "~" + dir[len(home):]
	}
	return dir
}

// snapFileName is a timestamped name, with -1, -2… when it is taken.
func snapFileName(dir string, now time.Time) string {
	base := "oh-my-toys-" + now.Format("20060102-150405")
	for i := 0; ; i++ {
		name := base + ".png"
		if i > 0 {
			name = fmt.Sprintf("%s-%d.png", base, i)
		}
		if _, err := os.Stat(filepath.Join(dir, name)); os.IsNotExist(err) {
			return name
		}
	}
}

// snapImage renders the selection: each display's part scaled to the
// selection's scale, which is the scale of the display the drag began on.
func (a *App) snapImage() (*image.RGBA, error) {
	s := a.snap
	if s == nil || s.sel.Empty() {
		return nil, fmt.Errorf("no selection")
	}
	scale := 1.0
	if ov := s.overlayAt(s.anchor); ov != nil {
		scale = ov.pixelsPerPoint()
	}
	out := image.NewRGBA(image.Rect(0, 0,
		int(float64(s.sel.Dx())*scale+0.5), int(float64(s.sel.Dy())*scale+0.5)))
	for _, ov := range s.overlays {
		part := s.sel.Intersect(ov.shot.Bounds)
		if part.Empty() {
			continue
		}
		k := ov.pixelsPerPoint() / scale
		dst := image.Rect(
			int(float64(part.Min.X-s.sel.Min.X)*scale+0.5), int(float64(part.Min.Y-s.sel.Min.Y)*scale+0.5),
			int(float64(part.Max.X-s.sel.Min.X)*scale+0.5), int(float64(part.Max.Y-s.sel.Min.Y)*scale+0.5))
		src := pixelRect(part, ov.shot.Bounds, ov.pixelsPerPoint())
		drawScaled(out, dst, ov.shot.Image, src, k)
	}
	snapDrawMarks(out, s.marks, s.sel, scale)
	return out, nil
}

func (s *snapSession) overlayAt(p image.Point) *snapOverlay {
	for _, ov := range s.overlays {
		if b := ov.shot.Bounds; p.X >= b.Min.X && p.X < b.Max.X && p.Y >= b.Min.Y && p.Y < b.Max.Y {
			return ov
		}
	}
	return nil
}

// pixelRect maps a rectangle of global points on a display to the
// display image's pixels, rounded outward so no edge pixel is lost.
func pixelRect(part, bounds image.Rectangle, ppp float64) image.Rectangle {
	px := func(v, origin int, up bool) int {
		f := float64(v-origin) * ppp
		if up {
			return int(math.Ceil(f))
		}
		return int(math.Floor(f))
	}
	return image.Rect(px(part.Min.X, bounds.Min.X, false), px(part.Min.Y, bounds.Min.Y, false),
		px(part.Max.X, bounds.Min.X, true), px(part.Max.Y, bounds.Min.Y, true))
}

// drawScaled draws src of srcImg into dst of dstImg, scaling by k when
// the display's pixel density differs from the selection's.
func drawScaled(dstImg *image.RGBA, dst image.Rectangle, srcImg *image.RGBA, src image.Rectangle, k float64) {
	if k == 1 {
		draw.Draw(dstImg, dst, srcImg, src.Min, draw.Src)
		return
	}
	xdraw.BiLinear.Scale(dstImg, dst, srcImg, src, xdraw.Over, nil)
}
