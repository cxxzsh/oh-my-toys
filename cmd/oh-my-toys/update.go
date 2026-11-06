package main

import (
	"context"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"time"

	"github.com/egoist/mygo"
)

// selfUpdate checks the feed of mygo.json's updates, installs a newer
// version and relaunches. OMT_UPDATE_DIR serves the feed from a directory
// instead, to try an update without publishing one.
func selfUpdate() {
	ctx := context.Background()
	if dir := os.Getenv("OMT_UPDATE_DIR"); dir != "" {
		mygo.Updater.SetSource(dirSource(dir))
	}
	t0 := time.Now()
	up, err := mygo.Updater.Check(ctx)
	if err != nil {
		log.Printf("[update] check: %v", err)
		return
	}
	if up == nil {
		log.Printf("[update] up to date (%s)", mygo.App.Version())
		return
	}
	log.Printf("[update] %s → %s, installing", mygo.App.Version(), up.Version)
	if err := up.Install(ctx, func(int64, int64) {}); err != nil {
		log.Printf("[update] install: %v", err)
		return
	}
	log.Printf("[update] installed in %s, relaunching", time.Since(t0).Round(time.Millisecond))
	mygo.App.Relaunch()
}

// dirSource serves the files `mygo build` wrote for the feed from a local
// directory, through MyGo's Updater.SetSource.
type dirSource string

func (d dirSource) Manifest(context.Context, bool) (io.ReadCloser, error) {
	return os.Open(filepath.Join(string(d), "update-"+runtime.GOOS+"-"+runtime.GOARCH+".json"))
}

func (d dirSource) Open(_ context.Context, url string) (io.ReadCloser, error) {
	return os.Open(filepath.Join(string(d), path.Base(url)))
}
