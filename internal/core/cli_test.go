package core

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	mygoi18n "github.com/egoist/mygo/i18n"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/tools/color"
	"github.com/cxxzsh/oh-my-toys/internal/tools/dupes"
	"github.com/cxxzsh/oh-my-toys/internal/tools/rename"
)

func newCLI(t *testing.T, tools ...tool.Tool) (CLI, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	reg, err := NewRegistry(tools...)
	if err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	return CLI{
		Registry: reg,
		Plans:    PlanStore{Dir: t.TempDir()},
		Msgs:     i18n.In("en"),
		Out:      &out,
		Err:      &errb,
	}, &out, &errb
}

func TestOutputLocale(t *testing.T) {
	if OutputLocale(true, "zh-Hans") != "" || OutputLocale(false, "zh-Hans") != "zh-Hans" {
		t.Fatal("json must stay in the source language")
	}
}

func TestColorPlanJSON(t *testing.T) {
	cli, out, errb := newCLI(t, color.New())
	code := cli.Run(context.Background(), []string{"color", "plan", "--json", "--color", "#fff"})
	if code != ExitOK {
		t.Fatal(code, errb)
	}
	var doc planDoc
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatal(err, out)
	}
	if doc.PlanID == "" || doc.Summary != "#ffffff" || doc.ApplyLabel != "" {
		t.Fatalf("doc %+v", doc)
	}
	if doc.Changes[0].Detail != "#ffffff" || doc.Changes[0].Tone != "normal" {
		t.Fatalf("changes %+v", doc.Changes)
	}
	if strings.Contains(out.String(), "白底") {
		t.Fatal("json was translated")
	}
}

func TestHumanFollowsLocale(t *testing.T) {
	mygoi18n.SetLocaleFunc(func() string { return "zh-Hans" })
	t.Cleanup(func() { mygoi18n.SetLocaleFunc(nil) })
	cli, out, errb := newCLI(t, color.New())
	cli.Msgs = i18n.In("zh-Hans")
	if code := cli.Run(context.Background(), []string{"color", "plan", "--color", "#fff"}); code != ExitOK {
		t.Fatal(code, errb)
	}
	if !strings.Contains(out.String(), "白底") {
		t.Fatalf("want Chinese label, got %s", out)
	}
}

func TestUsage(t *testing.T) {
	cli, _, errb := newCLI(t, color.New())
	if code := cli.Run(context.Background(), []string{"nope"}); code != ExitUsage {
		t.Fatalf("unknown tool %d", code)
	}
	if !strings.Contains(errb.String(), "nope") {
		t.Fatal(errb)
	}
	errb.Reset()
	if code := cli.Run(context.Background(), []string{"color", "plan"}); code != ExitUsage {
		t.Fatalf("missing flag %d %s", code, errb)
	}
	if code := cli.Run(context.Background(), []string{"color", "apply"}); code != ExitUsage {
		t.Fatalf("unknown action %d", code)
	}
}

func TestRenameDoesNotChangeUntilConfirmed(t *testing.T) {
	dir := renameDir(t)
	cli, out, errb := newCLI(t, rename.New())
	args := []string{"rename", "plan", "--folder", dir, "--find", "a", "--replace", "b"}
	if code := cli.Run(context.Background(), args); code != ExitOK {
		t.Fatal(code, errb)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.txt")); err != nil {
		t.Fatal("plan renamed the file")
	}
	id := planIDFrom(out.String())
	out.Reset()
	errb.Reset()
	if code := cli.Run(context.Background(), []string{"rename", "apply", "--plan", id}); code != ExitConfirm {
		t.Fatalf("non-tty apply %d %s", code, errb)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.txt")); err != nil {
		t.Fatal("applied without a terminal")
	}
	cli.Terminal = true
	cli.In = strings.NewReader("n\n")
	errb.Reset()
	if code := cli.Run(context.Background(), []string{"rename", "apply", "--plan", id}); code != ExitDenied {
		t.Fatalf("refused %d %s", code, errb)
	}
	if _, err := os.Stat(filepath.Join(dir, "b.txt")); !os.IsNotExist(err) {
		t.Fatal("refused apply renamed the file")
	}
	cli.In = strings.NewReader("y\n")
	if code := cli.Run(context.Background(), []string{"rename", "apply", "--plan", id}); code != ExitOK {
		t.Fatal(code, errb)
	}
	if _, err := os.Stat(filepath.Join(dir, "b.txt")); err != nil {
		t.Fatal("confirmed apply did not rename")
	}
	if _, err := os.Stat(filepath.Join(dir, "a.txt")); !os.IsNotExist(err) {
		t.Fatal("old name still exists")
	}
}

func TestApplyExpiredAndStale(t *testing.T) {
	dir := renameDir(t)
	now := time.Now()
	cli, out, errb := newCLI(t, rename.New())
	cli.Plans.Now = func() time.Time { return now }
	if code := cli.Run(context.Background(), []string{"rename", "plan", "--folder", dir, "--find", "a", "--replace", "b"}); code != ExitOK {
		t.Fatal(code, errb)
	}
	id := planIDFrom(out.String())
	now = now.Add(11 * time.Minute)
	cli.Terminal = true
	cli.In = strings.NewReader("y\n")
	errb.Reset()
	if code := cli.Run(context.Background(), []string{"rename", "apply", "--plan", id}); code != ExitError {
		t.Fatalf("expired %d %s", code, errb)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.txt")); err != nil {
		t.Fatal("expired plan renamed the file")
	}
	if !strings.Contains(errb.String(), "expired") {
		t.Fatal(errb)
	}
}

func TestApplyStale(t *testing.T) {
	dir := renameDir(t)
	cli, out, errb := newCLI(t, rename.New())
	if code := cli.Run(context.Background(), []string{"rename", "plan", "--folder", dir, "--find", "a", "--replace", "b"}); code != ExitOK {
		t.Fatal(code, errb)
	}
	id := planIDFrom(out.String())
	if err := os.WriteFile(filepath.Join(dir, "aa.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	cli.Terminal = true
	cli.In = strings.NewReader("y\n")
	errb.Reset()
	if code := cli.Run(context.Background(), []string{"rename", "apply", "--plan", id}); code != ExitError {
		t.Fatalf("stale %d %s", code, errb)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.txt")); err != nil {
		t.Fatal("stale plan renamed a file")
	}
	if _, err := os.Stat(filepath.Join(dir, "aa.txt")); err != nil {
		t.Fatal("stale plan touched the new file")
	}
}

func TestApplyWrongTool(t *testing.T) {
	cli, out, errb := newCLI(t, color.New(), rename.New())
	if code := cli.Run(context.Background(), []string{"color", "plan", "--color", "#fff"}); code != ExitOK {
		t.Fatal(code, errb)
	}
	id := planIDFrom(out.String())
	errb.Reset()
	code := cli.Run(context.Background(), []string{"rename", "apply", "--plan", id})
	if code != ExitError || !strings.Contains(errb.String(), "color") {
		t.Fatalf("wrong tool %d %s", code, errb)
	}
}

func TestDupesPlanChangesNothing(t *testing.T) {
	dir := t.TempDir()
	cli, out, errb := newCLI(t, dupes.New())
	code := cli.Run(context.Background(), []string{"dupes", "plan", "--json", "--folder", dir})
	if code != ExitOK {
		t.Fatal(code, errb)
	}
	var doc planDoc
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.PlanID == "" || doc.Destructive {
		t.Fatalf("doc %+v", doc)
	}
}

func TestSwitchHoldsUntilCancel(t *testing.T) {
	sw := &fakeSwitch{}
	cli, out, errb := newCLI(t, sw)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	code := cli.Run(ctx, []string{"fake", "start", "--json"})
	if code != ExitOK {
		t.Fatal(code, errb)
	}
	if sw.started != 1 || sw.stopped != 1 || sw.on {
		t.Fatalf("switch %+v", sw)
	}
	var doc statusDoc
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil || !doc.On {
		t.Fatal(err, out)
	}
}

func renameDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func planIDFrom(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	last := lines[len(lines)-1]
	if i := strings.LastIndex(last, "--plan "); i >= 0 {
		return strings.TrimSpace(last[i+len("--plan "):])
	}
	return strings.TrimSpace(last)
}

type fakeSwitch struct {
	on               bool
	started, stopped int
}

func (f *fakeSwitch) Manifest() tool.Manifest {
	return tool.Manifest{ID: "fake", Name: i18n.Messages.AppName, Summary: i18n.Messages.About, Kind: tool.KindSwitch}
}

func (f *fakeSwitch) Specs() []tool.Spec {
	return tool.SwitchSpecs(i18n.Messages.About, i18n.Messages.About, i18n.Messages.About)
}

func (f *fakeSwitch) Start(context.Context) error {
	f.started++
	f.on = true
	return nil
}

func (f *fakeSwitch) Stop() error {
	f.stopped++
	f.on = false
	return nil
}

func (f *fakeSwitch) Status() tool.Status {
	return tool.Status{On: f.on, Text: "on"}
}

// TestSwitchStartsForALengthAndEndsByItself is `omt <tool> start --for`
// without the app: the command holds the switch for that long, then stops
// it and returns, though nothing cancelled it.
func TestSwitchStartsForALengthAndEndsByItself(t *testing.T) {
	sw := &timedSwitch{}
	cli, _, errb := newCLI(t, sw)
	begun := time.Now()
	code := cli.Run(context.Background(), []string{"fake", "start", "--for", "30ms", "--json"})
	if code != ExitOK {
		t.Fatal(code, errb)
	}
	if want := []time.Duration{30 * time.Millisecond}; !slices.Equal(sw.lengths, want) {
		t.Errorf("ran for %v, want %v", sw.lengths, want)
	}
	if sw.stopped != 1 || sw.on {
		t.Errorf("not stopped when its length was over: %+v", sw)
	}
	if time.Since(begun) < 30*time.Millisecond {
		t.Errorf("returned after %v, before its length was over", time.Since(begun))
	}
}

func TestSwitchStartRejectsABadTimeOfDay(t *testing.T) {
	sw := &timedSwitch{}
	cli, _, errb := newCLI(t, sw)
	code := cli.Run(context.Background(), []string{"fake", "start", "--until", "25:00"})
	if code != ExitError || sw.on || !strings.Contains(errb.String(), i18n.HoldClock()) {
		t.Fatalf("code %d, on %v, stderr %q", code, sw.on, errb)
	}
}

// TestPlainSwitchHasNoLengthFlags: only a timed tool's start takes --for.
func TestPlainSwitchHasNoLengthFlags(t *testing.T) {
	cli, _, _ := newCLI(t, &fakeSwitch{})
	if code := cli.Run(context.Background(), []string{"fake", "start", "--for", "1h"}); code != ExitUsage {
		t.Fatalf("code %d, want %d", code, ExitUsage)
	}
}
