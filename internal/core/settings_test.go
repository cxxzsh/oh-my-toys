package core

import (
	"slices"
	"testing"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

func TestMoveWithinKeepsOtherGroups(t *testing.T) {
	var s Settings
	files := []string{"rename", "dupes"}
	s.MoveWithin(files, []int{1}, 0) // dupes before rename
	got := s.Ordered([]string{"awake", "rename", "dupes", "ports"})
	want := []string{"dupes", "rename", "awake", "ports"}
	if !slices.Equal(got, want) {
		t.Errorf("order %v, want %v", got, want)
	}
	// Moving inside another group leaves the first group's order alone.
	s.MoveWithin([]string{"awake"}, []int{0}, 1)
	if got := s.Ordered(files); !slices.Equal(got, []string{"dupes", "rename"}) {
		t.Errorf("files order after moving another group: %v", got)
	}
}

func TestMoveWithinTable(t *testing.T) {
	group := []string{"a", "b", "c", "d"}
	for _, c := range []struct {
		rows []int
		to   int
		want []string
	}{
		{[]int{0}, 3, []string{"b", "c", "a", "d"}},
		{[]int{3}, 0, []string{"d", "a", "b", "c"}},
		{[]int{0}, 4, []string{"b", "c", "d", "a"}},
		{[]int{1, 2}, 0, []string{"b", "c", "a", "d"}},
		{[]int{1}, 2, []string{"a", "b", "c", "d"}},
	} {
		var s Settings
		s.MoveWithin(group, c.rows, c.to)
		if got := s.Ordered(group); !slices.Equal(got, c.want) {
			t.Errorf("move %v to %d: %v, want %v", c.rows, c.to, got, c.want)
		}
	}
}

func TestResetKeepsLanguage(t *testing.T) {
	s := Settings{Pinned: []string{"x"}, Order: []string{"b", "a"}, Collapsed: []string{"file"}, Language: "zh-Hans"}
	s.Reset(Settings{Pinned: []string{"a", "b"}})
	if !slices.Equal(s.Pinned, []string{"a", "b"}) || s.Order != nil || s.Collapsed != nil || s.Language != "zh-Hans" {
		t.Errorf("after reset: %+v", s)
	}
}

func TestMoveSectionKeepsTheRest(t *testing.T) {
	all := []string{"pinned", "system", "file", "dev"}
	var s Settings
	s.MoveSection(all, 2, 1) // file before system
	got := s.OrderedSections(all)
	if want := []string{"pinned", "file", "system", "dev"}; !slices.Equal(got, want) {
		t.Errorf("sections %v, want %v", got, want)
	}
	s.MoveSection(got, 0, 4) // pinned to the end
	if got := s.OrderedSections(all); !slices.Equal(got, []string{"file", "system", "dev", "pinned"}) {
		t.Errorf("after moving pinned last: %v", got)
	}
	s.Reset(Settings{})
	if s.Sections != nil {
		t.Error("reset kept the section order")
	}
}

// TestSchedulesSurviveResetAndRestart: a plan is not part of the sidebar,
// so resetting the sidebar leaves it, and it comes back from the file.
func TestSchedulesSurviveResetAndRestart(t *testing.T) {
	plan := tool.Schedule{Enabled: true, Display: true, Segments: []tool.Segment{
		{Days: tool.Workdays, From: 9 * 60, To: 12 * 60}, {Days: tool.AllDays, From: 22 * 60, To: 6 * 60},
	}}
	var s Settings
	s.SetSchedule("awake", plan)
	plan.Segments[0].From = 0 // the caller's slice is not the settings'
	s.Reset(Settings{})

	store := NewStore(t.TempDir())
	if err := store.Save(s); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load(Settings{})
	if err != nil {
		t.Fatal(err)
	}
	want := tool.Schedule{Enabled: true, Display: true, Segments: []tool.Segment{
		{Days: tool.Workdays, From: 9 * 60, To: 12 * 60}, {Days: tool.AllDays, From: 22 * 60, To: 6 * 60},
	}}
	if !got.Schedules["awake"].Equal(want) {
		t.Errorf("plan after reset and reload: %+v, want %+v", got.Schedules["awake"], want)
	}
}

// TestDisplayChoiceSurvivesResetAndRestart: whether a tool keeps the display
// on is not part of the sidebar, and a choice to let it sleep is kept as
// firmly as one to keep it on.
func TestDisplayChoiceSurvivesResetAndRestart(t *testing.T) {
	var s Settings
	s.SetDisplay("awake", false)
	s.Reset(Settings{})
	store := NewStore(t.TempDir())
	if err := store.Save(s); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load(Settings{})
	if err != nil {
		t.Fatal(err)
	}
	if on, kept := got.Displays["awake"]; !kept || on {
		t.Errorf("display choice after reset and reload: %v, kept %v, want false, kept", on, kept)
	}
}

// TestPresetsAreSavedOnceAndKept: a length is saved once, in the order it
// was saved, removing one that is not there changes nothing, and they come
// back from the file after a sidebar reset.
func TestPresetsAreSavedOnceAndKept(t *testing.T) {
	var s Settings
	s.RemovePreset("awake", time.Hour) // nothing saved yet
	s.AddPreset("awake", 90*time.Minute)
	s.AddPreset("awake", 45*time.Minute)
	s.AddPreset("awake", 90*time.Minute)
	s.AddPreset("awake", 0)
	s.AddPreset("awake", 20*time.Second)
	s.Reset(Settings{})

	store := NewStore(t.TempDir())
	if err := store.Save(s); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load(Settings{})
	if err != nil {
		t.Fatal(err)
	}
	if want := []time.Duration{90 * time.Minute, 45 * time.Minute}; !slices.Equal(got.Presets("awake"), want) {
		t.Fatalf("presets %v, want %v", got.Presets("awake"), want)
	}
	got.RemovePreset("awake", 90*time.Minute)
	got.RemovePreset("awake", 45*time.Minute)
	if len(got.Presets("awake")) != 0 || len(got.SavedPresets) != 0 {
		t.Errorf("after removing both: %v", got.SavedPresets)
	}
}
