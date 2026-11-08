package awake

import (
	"testing"
	"time"
)

// newTestTool makes a tool whose system calls are counted: held is how
// many keep-awake requests are still open, and displays what they asked.
func newTestTool(t *testing.T) (tool *Tool, held *int, displays *[]bool) {
	t.Helper()
	held, displays = new(int), new([]bool)
	old := keepAwake
	keepAwake = func(_ string, display bool) func() {
		*held++
		*displays = append(*displays, display)
		return func() { *held-- }
	}
	t.Cleanup(func() { keepAwake = old })
	return New(func() {}), held, displays
}

func runAction(t *testing.T, tl *Tool, id string) {
	t.Helper()
	for _, a := range tl.Manifest().Actions {
		if a.ID == id {
			if err := a.Run(t.Context()); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatalf("no quick action %q", id)
}

func TestStatusNamesTheModeAndItsEnd(t *testing.T) {
	tl, held, _ := newTestTool(t)
	if st := tl.Status(); st.On || st.Active != "" || !st.Since.IsZero() {
		t.Fatalf("off, but status is %+v", st)
	}

	runAction(t, tl, idTwoHours)
	st := tl.Status()
	if !st.On || st.Active != idTwoHours {
		t.Fatalf("after 2h: %+v", st)
	}
	if got := st.Until.Sub(st.Since); got != 2*time.Hour {
		t.Errorf("runs for %v, want 2h", got)
	}

	runAction(t, tl, idUntilOff)
	if st := tl.Status(); st.Active != idUntilOff || !st.Until.IsZero() {
		t.Errorf("until turned off, but status is %+v", st)
	}
	if *held != 1 {
		t.Errorf("%d requests held after switching mode, want 1: the old one must be released", *held)
	}

	runAction(t, tl, idOff)
	if st := tl.Status(); st.On || st.Active != "" {
		t.Errorf("after off: %+v", st)
	}
	if *held != 0 {
		t.Errorf("%d requests held after off", *held)
	}
}

// TestStartResumesTheLastMode is what the tray switch and the page's
// switch do: turn on with the settings used last, not with the defaults.
func TestStartResumesTheLastMode(t *testing.T) {
	tl, _, _ := newTestTool(t)
	runAction(t, tl, idTwoHours)
	if err := tl.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := tl.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if st := tl.Status(); st.Active != idTwoHours || st.Until.Sub(st.Since) != 2*time.Hour {
		t.Errorf("resumed %+v, want the 2 hours", st)
	}
}

// TestDisplayIsChosenApartFromTheMode: every mode, typed lengths too, keeps
// the display on as the choice says, and a change reaches the run in
// progress without ending it or moving its end.
func TestDisplayIsChosenApartFromTheMode(t *testing.T) {
	tl, held, displays := newTestTool(t)
	tl.SetDisplay(false)
	runAction(t, tl, idHour)
	if err := tl.StartFor(t.Context(), time.Hour); err != nil {
		t.Fatal(err)
	}
	if got := *displays; len(got) != 2 || got[0] || got[1] {
		t.Fatalf("display asked %v: the choice was to let it sleep, in both runs", got)
	}

	before := tl.Status()
	tl.SetDisplay(true)
	after := tl.Status()
	if *held != 1 {
		t.Errorf("%d requests held after the change, want the one run", *held)
	}
	if got := *displays; !got[len(got)-1] {
		t.Errorf("display asked %v: the change did not reach the run", got)
	}
	if !after.Until.Equal(before.Until) || !after.Since.Equal(before.Since) || after.Active != before.Active {
		t.Errorf("the run changed from %+v to %+v", before, after)
	}
	if after.Detail != displayDetail(true) || before.Detail != displayDetail(false) {
		t.Errorf("details %q then %q do not follow the choice", before.Detail, after.Detail)
	}
}

func TestOnlyOffStops(t *testing.T) {
	tl, _, _ := newTestTool(t)
	for _, a := range tl.Manifest().Actions {
		if a.Stops != (a.ID == idOff) {
			t.Errorf("action %q: Stops is %v", a.ID, a.Stops)
		}
	}
}

// TestStartForRunsThatLong is a length a person typed: it has no quick
// action, so Status calls it custom, and turning the tool off and on again
// brings back the same length.
func TestStartForRunsThatLong(t *testing.T) {
	tl, held, _ := newTestTool(t)
	if err := tl.StartFor(t.Context(), 45*time.Minute); err != nil {
		t.Fatal(err)
	}
	st := tl.Status()
	if st.Active != idCustom || st.Until.Sub(st.Since) != 45*time.Minute {
		t.Fatalf("after StartFor: %+v", st)
	}
	if err := tl.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := tl.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if st := tl.Status(); st.Active != idCustom || st.Until.Sub(st.Since) != 45*time.Minute {
		t.Errorf("resumed as %+v, want the same 45 minutes", st)
	}
	if *held != 1 {
		t.Errorf("%d requests held, want 1", *held)
	}
}
