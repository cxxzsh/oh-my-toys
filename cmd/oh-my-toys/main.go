// Command oh-my-toys is the desktop app: a box of small tools, with a main
// window, a tray panel and a command palette.
//
//	go run ./cmd/oh-my-toys [-dev] [-material sidebar|glass|mica|none]
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo"

	"github.com/cxxzsh/oh-my-toys/internal/core"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/tools/awake"
	"github.com/cxxzsh/oh-my-toys/internal/tools/color"
	"github.com/cxxzsh/oh-my-toys/internal/tools/devtools"
	"github.com/cxxzsh/oh-my-toys/internal/tools/dupes"
	"github.com/cxxzsh/oh-my-toys/internal/tools/ports"
	"github.com/cxxzsh/oh-my-toys/internal/tools/rename"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
	"github.com/cxxzsh/oh-my-toys/internal/ui/shell"
)

// fakeTools is how many made-up tools -dev adds.
const fakeTools = 500

type flags struct {
	dev, show, exitAfterFirstFrame, update bool
	material, shortcuts, screenshots       string
}

func parseFlags() flags {
	var f flags
	flag.BoolVar(&f.dev, "dev", false, "add 500 made-up tools and a crash test, and show measurements")
	flag.BoolVar(&f.show, "show", true, "show the main window at start")
	flag.StringVar(&f.material, "material", defaultMaterial(), "main window material: sidebar, glass, mica or none")
	flag.StringVar(&f.shortcuts, "shortcut", "Alt+Space,Alt+Shift+Space,CmdOrCtrl+Shift+K", "global shortcuts to try, in order")
	flag.BoolVar(&f.exitAfterFirstFrame, "exit-after-first-frame", false, "print the time to the first frame and quit")
	flag.StringVar(&f.screenshots, "screenshots", "", "capture screenshots into this directory, then quit")
	flag.BoolVar(&f.update, "update", false, "check for an update at start, install it and relaunch")
	flag.Parse()
	return f
}

func defaultMaterial() string {
	switch runtime.GOOS {
	case "darwin":
		return "sidebar"
	case "windows":
		return "mica"
	}
	return "none"
}

// tools is the one list of the app's tools, in the order they are shown.
func tools(events *core.Events, dev bool) []tool.Tool {
	list := []tool.Tool{awake.New(events.Changed), rename.New(), dupes.New(), ports.New(), color.New()}
	if dev {
		list = append(list, devtools.Crash{})
		list = append(list, devtools.Fakes(fakeTools, events.Changed)...)
	}
	return list
}

func main() {
	started := time.Now()
	f := parseFlags()
	portable := usePortableData()
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	events := &core.Events{}
	reg, err := core.NewRegistry(tools(events, f.dev)...)
	if err != nil {
		log.Fatal(err)
	}
	if err := ds.RegisterFonts(); err != nil {
		log.Print(err) // the system's fonts stand in
	}
	mygo.App.SetName("oh-my-toys")
	app := shell.New(shell.Options{
		Registry:      reg,
		Runner:        core.NewRunner(events.Changed),
		Events:        events,
		Store:         core.NewStore(dataDir()),
		Defaults:      core.Settings{Pinned: []string{awake.ID, rename.ID, dupes.ID, ports.ID, color.ID}},
		Dev:           f.dev,
		Portable:      portable,
		Material:      f.material,
		Shortcuts:     strings.Split(f.shortcuts, ","),
		ShowMain:      f.show,
		StartedAt:     started,
		ScreenshotDir: f.screenshots,
		OnFirstFrame:  firstFrame(f.exitAfterFirstFrame),
	})
	mygo.App.OnSecondInstance(func([]string, string) { app.Post(app.ShowMain) })
	ipcCtx, stopIPC := context.WithCancel(context.Background())
	go serveIPC(ipcCtx, reg)
	mygo.App.WhenReady(app.Start)
	switch {
	case f.update && portable:
		// Replacing the app in place would leave its folder half old, half
		// new: a portable app is updated by unzipping a newer one.
		log.Print("[update] skipped: a portable app does not update itself")
	case f.update:
		mygo.App.WhenReady(func() { go selfUpdate() })
	}
	err = mygo.App.Run()
	stopIPC()
	if err != nil {
		log.Fatal(err)
	}
}

func firstFrame(exit bool) func(time.Duration) {
	return func(d time.Duration) {
		if exit {
			fmt.Printf("first_frame_ms=%.1f\n", float64(d.Microseconds())/1000)
			os.Exit(0)
		}
	}
}

// usePortableData points MyGo's data, cache and log folders at the data
// folder next to the executable when the app is portable, and reports
// whether it is. It runs before anything reads or writes those folders.
func usePortableData() bool {
	exe, err := os.Executable()
	if err != nil {
		log.Print(err)
		return false
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	dir, ok := core.PortableDir(exe)
	if !ok {
		return false
	}
	mygo.App.SetPath(mygo.PathUserData, dir)
	mygo.App.SetPath(mygo.PathCache, filepath.Join(dir, "cache"))
	mygo.App.SetPath(mygo.PathLogs, filepath.Join(dir, "logs"))
	return true
}

// dataDir is where the settings live: the app's user data directory, or a
// temporary one when the system has none.
func dataDir() string {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		log.Print(err)
		return os.TempDir()
	}
	return dir
}

func serveIPC(ctx context.Context, reg *core.Registry) {
	ln, err := core.ListenSwitch(ctx)
	if err != nil {
		log.Printf("ipc: %v", err)
		return
	}
	if err := core.ServeSwitch(ctx, reg, ln); err != nil && ctx.Err() == nil {
		log.Printf("ipc: %v", err)
	}
}
