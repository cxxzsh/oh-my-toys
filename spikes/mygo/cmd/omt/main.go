// Command omt runs the same tools as the desktop app from a terminal.
//
//	omt [--json] <tool> <action> [--flag value]
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"syscall"

	mygoi18n "github.com/egoist/mygo/i18n"

	"github.com/cxxzsh/oh-my-toys/spikes/mygo/internal/core"
	"github.com/cxxzsh/oh-my-toys/spikes/mygo/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/spikes/mygo/internal/tool"
	"github.com/cxxzsh/oh-my-toys/spikes/mygo/internal/tools/awake"
	"github.com/cxxzsh/oh-my-toys/spikes/mygo/internal/tools/color"
	"github.com/cxxzsh/oh-my-toys/spikes/mygo/internal/tools/dupes"
	"github.com/cxxzsh/oh-my-toys/spikes/mygo/internal/tools/ports"
	"github.com/cxxzsh/oh-my-toys/spikes/mygo/internal/tools/rename"
)

func main() {
	args := os.Args[1:]
	locale := core.OutputLocale(slices.Contains(args, "--json"), systemLocale())
	mygoi18n.SetLocaleFunc(func() string { return locale })
	code := run(args, locale)
	os.Exit(code)
}

func run(args []string, locale string) int {
	reg, err := core.NewRegistry(tools()...)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return core.ExitError
	}
	dir, err := planDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return core.ExitError
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return core.CLI{
		Registry: reg,
		Plans:    core.PlanStore{Dir: dir},
		Msgs:     i18n.In(locale),
		In:       os.Stdin,
		Out:      os.Stdout,
		Err:      os.Stderr,
		Terminal: isTerminal(os.Stdin),
	}.Run(ctx, args)
}

func tools() []tool.Tool {
	return []tool.Tool{awake.New(func() {}), rename.New(), dupes.New(), ports.New(), color.New()}
}

func planDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if dir, ok := core.PortableDir(exe); ok {
		return filepath.Join(dir, "plans"), nil
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cache, "oh-my-toys", "plans"), nil
}
