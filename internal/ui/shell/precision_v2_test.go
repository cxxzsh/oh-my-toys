package shell

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	mygoi18n "github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/core"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/tools/color"
	"github.com/cxxzsh/oh-my-toys/internal/tools/devtools"
	"github.com/cxxzsh/oh-my-toys/internal/tools/dupes"
	"github.com/cxxzsh/oh-my-toys/internal/tools/ports"
	"github.com/cxxzsh/oh-my-toys/internal/tools/rename"
	"github.com/cxxzsh/oh-my-toys/internal/tools/screenshot"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

func TestParseDirectCommand(t *testing.T) {
	m := &modeSwitch{}
	reg, err := core.NewRegistry(m)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 11, 14, 0, 0, 0, time.Local)

	d, ok := parseDirectCommand(reg, modeToolID+" 45m", now)
	if !ok || d.dur != 45*time.Minute || d.flag != tool.PropFor {
		t.Fatalf("45m = (%+v, %v), want 45m --for", d, ok)
	}

	d, ok = parseDirectCommand(reg, modeToolID+" 15:30", now)
	if !ok || d.dur != 90*time.Minute || d.flag != tool.PropUntil {
		t.Fatalf("15:30 = (%+v, %v), want 90m --until", d, ok)
	}

	for _, bad := range []string{"", modeToolID, modeToolID + " -5m", modeToolID + " 99:99", "unknown 30m"} {
		if _, ok := parseDirectCommand(reg, bad, now); ok {
			t.Errorf("parseDirectCommand(%q) succeeded, want false", bad)
		}
	}
}

func TestSplitCommonAffix(t *testing.T) {
	cases := []struct {
		from, to                   string
		prefix, delMid, addMid, sf string
	}{
		{"IMG_4021_draft.pdf", "IMG_4021_final.pdf", "IMG_4021_", "draft", "final", ".pdf"},
		{"aaaa.txt", "aa.txt", "aa", "aa", "", ".txt"},
		{"报告_旧版_2026.md", "报告_定稿_2026.md", "报告_", "旧版", "定稿", "_2026.md"},
		{"same.go", "same.go", "same.go", "", "", ""},
	}
	for _, tc := range cases {
		p, del, add, sf := splitCommonAffix(tc.from, tc.to)
		if p != tc.prefix || del != tc.delMid || add != tc.addMid || sf != tc.sf {
			t.Errorf("splitCommonAffix(%q, %q) = (%q, %q, %q, %q), want (%q, %q, %q, %q)",
				tc.from, tc.to, p, del, add, sf, tc.prefix, tc.delMid, tc.addMid, tc.sf)
		}
	}
}

func TestIsExposedBind(t *testing.T) {
	for _, exposed := range []string{"0.0.0.0:3000", "*:8080", "[::]:443", "::"} {
		if !isExposedBind(exposed) {
			t.Errorf("isExposedBind(%q) = false, want true", exposed)
		}
	}
	for _, local := range []string{"127.0.0.1:3000", "[::1]:3000", "::1", "localhost:8080"} {
		if isExposedBind(local) {
			t.Errorf("isExposedBind(%q) = true, want false", local)
		}
	}
}

func TestSnapStepBackTransitions(t *testing.T) {
	a := &App{snap: &snapSession{
		sel:  image.Rect(10, 10, 100, 100),
		tool: snapToolArrow,
		text: &snapText{},
	}}

	a.snapStepBack()
	if a.snap.text != nil || a.snap.tool != snapToolArrow || a.snap.sel.Empty() {
		t.Fatalf("step 1 (clear text): %+v", a.snap)
	}

	a.snapStepBack()
	if a.snap.tool != snapToolNone || a.snap.sel.Empty() {
		t.Fatalf("step 2 (clear tool): %+v", a.snap)
	}

	a.snapStepBack()
	if !a.snap.sel.Empty() || a.snap == nil {
		t.Fatalf("step 3 (clear selection): %+v", a.snap)
	}

	a.snapStepBack()
	if a.snap != nil {
		t.Fatalf("step 4 (cancel session): got %+v, want nil", a.snap)
	}
}

func TestDroppedFolderRoutesToPathAction(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := droppedFolder(dir); got != dir {
		t.Fatalf("droppedFolder(dir) = %q, want %q", got, dir)
	}
	if got := droppedFolder(file); got != dir {
		t.Fatalf("droppedFolder(file) = %q, want %q", got, dir)
	}
	a := newRenderApp(t, true, pageHome)
	act, field, ok := a.targetPathAction()
	if !ok || act.Manifest().ID != "rename" || field.Name != "folder" {
		t.Fatalf("targetPathAction = (%v, %+v, %v), want rename/folder", act, field, ok)
	}
}

func TestRenderPrecisionPagesHiDPI(t *testing.T) {
	dir := os.Getenv("OMT_SHOTS")
	if dir == "" {
		return
	}
	if err := ds.RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, dark := range []bool{true, false} {
		renderThemePages(t, dir, dark)
	}
	renderZhAndResponsive(t, dir)
}

func renderThemePages(t *testing.T, dir string, dark bool) {
	t.Helper()
	suffix := "light"
	if dark {
		suffix = "dark"
	}
	for _, page := range []string{pageHome, modeToolID, "ports", "rename", "dupes", "color", "screenshot", devtools.TrafficID, pageSettings} {
		a := newRenderApp(t, dark, page)
		seedRenderPage(t, a, page)
		savePNG(t, filepath.Join(dir, "native-"+page+"-"+suffix+".png"), ui.Render(a.mainView, ds.WindowWidth, ds.WindowHeight, 2))
	}
	a := newRenderApp(t, dark, "ports")
	a.sideCompact = true
	seedRenderPage(t, a, "ports")
	savePNG(t, filepath.Join(dir, "native-compact-rail-"+suffix+".png"), ui.Render(a.mainView, ds.WindowWidth, ds.WindowHeight, 2))

	planApp := newRenderApp(t, dark, modeToolID)
	planApp.switchForm(modeToolID).tab = tabPlan
	savePNG(t, filepath.Join(dir, "native-switch-plan-"+suffix+".png"), ui.Render(planApp.mainView, ds.WindowWidth, ds.WindowHeight, 2))

	palApp := newRenderApp(t, dark, pageHome)
	palApp.pal.open = true
	palApp.pal.query = modeToolID + " 45m"
	savePNG(t, filepath.Join(dir, "native-palette-direct-"+suffix+".png"), ui.Render(palApp.mainView, ds.WindowWidth, ds.WindowHeight, 2))

	dlgApp := newRenderApp(t, dark, "ports")
	seedRenderPage(t, dlgApp, "ports")
	dlgApp.askDialog(comp.DialogOpts{
		Title:       "End node (PID 4821)?",
		Description: "Unsaved work in this process will be lost.",
		Items:       []comp.DialogItem{{Text: "node  :3000  pid 4821"}},
		Cancel:      "Cancel",
		Confirm:     "End 1 Process",
		Destructive: true,
	})
	savePNG(t, filepath.Join(dir, "native-dialog-"+suffix+".png"), ui.Render(dlgApp.mainView, ds.WindowWidth, ds.WindowHeight, 2))

	wideTraffic := newRenderApp(t, dark, devtools.TrafficID)
	savePNG(t, filepath.Join(dir, "native-traffic-wide-"+suffix+".png"), ui.Render(wideTraffic.mainView, 1680, 920, 2))

	trayApp := newRenderApp(t, dark, pageHome)
	savePNG(t, filepath.Join(dir, "native-tray-"+suffix+".png"), ui.Render(trayApp.trayView, ds.TrayWidth, 620, 2))
}

func renderZhAndResponsive(t *testing.T, dir string) {
	t.Helper()
	narrowTraffic := newRenderApp(t, true, devtools.TrafficID)
	savePNG(t, filepath.Join(dir, "native-narrow-880-traffic-dark.png"), ui.Render(narrowTraffic.mainView, ds.WindowMinWidth, ds.WindowMinHeight, 2))

	narrowPorts := newRenderApp(t, true, "ports")
	seedRenderPage(t, narrowPorts, "ports")
	savePNG(t, filepath.Join(dir, "native-narrow-880-ports-dark.png"), ui.Render(narrowPorts.mainView, ds.WindowMinWidth, ds.WindowMinHeight, 2))

	mygoi18n.SetLocaleFunc(func() string { return "zh-Hans" })
	defer mygoi18n.SetLocaleFunc(nil)

	for _, page := range []string{pageHome, devtools.TrafficID, "color", pageSettings} {
		zhApp := newRenderApp(t, true, page)
		seedRenderPage(t, zhApp, page)
		savePNG(t, filepath.Join(dir, "native-zh-"+page+"-dark.png"), ui.Render(zhApp.mainView, ds.WindowWidth, ds.WindowHeight, 2))
	}
	zhPlan := newRenderApp(t, false, modeToolID)
	zhPlan.switchForm(modeToolID).tab = tabPlan
	savePNG(t, filepath.Join(dir, "native-zh-switch-plan-light.png"), ui.Render(zhPlan.mainView, ds.WindowWidth, ds.WindowHeight, 2))
}

func seedRenderPage(t *testing.T, a *App, page string) {
	t.Helper()
	switch page {
	case "color":
		ct := color.New()
		plan, err := ct.Plan(t.Context(), tool.Input{"color": "#2563eb"})
		if err != nil {
			t.Fatal(err)
		}
		hex := "#2563eb"
		s := a.action("color")
		s.started = true
		s.input["color"] = hex
		s.inputs["color"] = &hex
		s.plan = plan
	case "ports":
		s := a.action("ports")
		s.started = true
		s.plan = tool.Plan{
			Summary: "4 listening",
			Rows: []tool.Row{
				{Value: "3000", Cells: []string{":3000", "node", "4821", "127.0.0.1:3000", ""}, Process: &tool.Process{Path: "/usr/local/bin/node"}},
				{Value: "8080", Cells: []string{":8080", "api-server", "5109", "0.0.0.0:8080", ""}, Process: &tool.Process{Path: "/usr/local/bin/api-server"}},
				{Value: "5432", Cells: []string{":5432", "postgres", "1204", "127.0.0.1:5432", ""}, Process: &tool.Process{Path: "/opt/homebrew/bin/postgres"}},
				{Value: "5353", Cells: []string{":5353", "mDNSResponder", "412", "*:5353", ""}, Process: &tool.Process{Path: "/usr/sbin/mDNSResponder", System: true}},
			},
		}
	case "rename":
		folder, find, rep := "~/Downloads/Q4-Invoices", "draft", "final"
		s := a.action("rename")
		s.started = true
		s.input = tool.Input{"folder": folder, "find": find, "replace": rep}
		s.inputs = map[string]*string{"folder": &folder, "find": &find, "replace": &rep}
		s.plan = tool.Plan{
			Summary:    "3 files to rename, 1 conflict skipped",
			ApplyLabel: "Rename 3 Files",
			Columns:    []string{"Current", "New", "State"},
			Rows: []tool.Row{
				{Cells: []string{"INV-2041_Acme_draft.pdf", "INV-2041_Acme_final.pdf", "Rename"}, Selectable: true},
				{Cells: []string{"INV-2043_Globex_draft.pdf", "INV-2043_Globex_final.pdf", "Rename"}, Selectable: true},
				{Cells: []string{"Receipt_Initech_draft.pdf", "Receipt_Initech_final.pdf", "Rename"}, Selectable: true},
				{Cells: []string{"Summary_2026_draft.pdf", "Summary_2026_final.pdf", "Exists"}, Tone: tool.ToneWarning},
			},
		}
		s.selected = []bool{true, true, true, false}
	case "dupes":
		s := a.action("dupes")
		s.started = true
		s.plan = tool.Plan{
			Summary:     "2 groups found, 418.6 MB reclaimable",
			ApplyLabel:  "Move 4 Copies to Trash",
			Destructive: true,
			Rows: []tool.Row{
				{Title: "INV-2041_Acme.pdf", Detail: "KEEP · 1.2 MB", Value: "INV-2041_Acme.pdf", Tone: tool.ToneSuccess, Group: "1.2 MB each · 3 copies"},
				{Title: "INV-2041_Acme (1).pdf", Detail: "Move to Trash · 1.2 MB", Value: "INV-2041_Acme (1).pdf", Tone: tool.ToneWarning, Selectable: true},
				{Title: "INV-2041_Acme (2).pdf", Detail: "Move to Trash · 1.2 MB", Value: "INV-2041_Acme (2).pdf", Tone: tool.ToneWarning, Selectable: true},
				{Title: "Banner@2x.png", Detail: "KEEP · 68 MB", Value: "Banner@2x.png", Tone: tool.ToneSuccess, Group: "68 MB each · 3 copies"},
				{Title: "Banner@2x (1).png", Detail: "Move to Trash · 68 MB", Value: "Banner@2x (1).png", Tone: tool.ToneWarning, Selectable: true},
				{Title: "Banner@2x (2).png", Detail: "Move to Trash · 68 MB", Value: "Banner@2x (2).png", Tone: tool.ToneWarning, Selectable: true},
			},
		}
		s.selected = []bool{false, true, true, false, true, true}
	case "screenshot":
		a.snapRecentsDir = a.snapDir()
		a.snapRecents = []recentFile{
			{name: "oh-my-toys-20261111-142012.png", size: 284 * 1024},
			{name: "oh-my-toys-20261111-135804.png", size: 612 * 1024},
			{name: "oh-my-toys-20261111-110459.png", size: 1480 * 1024},
		}
	}
}

func newRenderApp(t *testing.T, dark bool, page string) *App {
	t.Helper()
	events := &core.Events{}
	now := time.Now()
	aw := &modeSwitch{
		st: tool.Status{On: true, Active: "short", Since: now, Until: now.Add(45 * time.Minute), Text: "00:45:00"},
		sched: tool.Schedule{
			Enabled:  true,
			Segments: []tool.Segment{{Days: tool.Workdays, From: 9 * 60, To: 18 * 60}},
		},
	}
	reg, err := core.NewRegistry(
		aw,
		ports.New(),
		rename.New(),
		dupes.New(),
		color.New(),
		screenshot.New(),
		devtools.NewTraffic(),
	)
	if err != nil {
		t.Fatal(err)
	}
	a := New(Options{
		Registry:      reg,
		Runner:        core.NewRunner(nil),
		Events:        events,
		Store:         core.NewStore(t.TempDir()),
		Defaults:      core.Settings{Pinned: []string{modeToolID, devtools.TrafficID, "ports", "rename", "color"}},
		SnapShortcut:  "CmdOrCtrl+Shift+4",
		ScreenAllowed: func() bool { return false },
	})
	a.forceDark = &dark
	a.page = page
	return a
}

func savePNG(t *testing.T, path string, img image.Image) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}
