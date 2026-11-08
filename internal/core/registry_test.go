package core

import (
	"testing"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// panickingSwitch is a tool whose Status panics: it must not take the other
// tools down with it when the app quits.
type panickingSwitch struct{ fakeSwitch }

func (p *panickingSwitch) Manifest() tool.Manifest {
	return tool.Manifest{ID: "boom", Name: i18n.Messages.AppName, Summary: i18n.Messages.About, Kind: tool.KindSwitch}
}

func (p *panickingSwitch) Status() tool.Status { panic("status exploded") }

// TestStopAllKeepsGoingAfterAPanic: StopAll is the last thing the app does,
// and a proxy left behind cuts the person off the network, so one broken
// tool must not stop the others from stopping.
func TestStopAllKeepsGoingAfterAPanic(t *testing.T) {
	running := &fakeSwitch{on: true}
	reg, err := NewRegistry(&panickingSwitch{}, running)
	if err != nil {
		t.Fatal(err)
	}
	reg.StopAll()
	if running.stopped != 1 || running.on {
		t.Fatalf("the tool after the panicking one was not stopped: %+v", running)
	}
}

// secondSwitch is another tool of the fakeSwitch kind, under another ID.
type secondSwitch struct{ fakeSwitch }

func (s *secondSwitch) Manifest() tool.Manifest {
	return tool.Manifest{ID: "second", Name: i18n.Messages.AppName, Summary: i18n.Messages.About, Kind: tool.KindSwitch}
}

// TestStopAllStopsEveryRunningTool keeps the ordinary case pinned.
func TestStopAllStopsEveryRunningTool(t *testing.T) {
	first := &fakeSwitch{on: true}
	idle := &secondSwitch{}
	reg, err := NewRegistry(first, idle)
	if err != nil {
		t.Fatal(err)
	}
	reg.StopAll()
	if first.stopped != 1 || first.on {
		t.Errorf("the running tool was not stopped: %+v", first)
	}
	if idle.stopped != 0 {
		t.Errorf("a tool that was off was stopped: %+v", idle)
	}
}
