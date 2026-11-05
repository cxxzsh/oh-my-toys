package ports

import (
	"context"
	"errors"
	"testing"

	"github.com/cxxzsh/oh-my-toys/spikes/mygo/internal/tool"
)

func TestPlanListsListeners(t *testing.T) {
	p, err := New().Plan(context.Background(), tool.Input{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Summary == "" {
		t.Error("no summary")
	}
	if p.ApplyLabel != "" {
		t.Error("a plan of every port must not end processes")
	}
}

func TestBadPort(t *testing.T) {
	for _, in := range []string{"abc", "0", "70000", "-1"} {
		_, err := New().Plan(context.Background(), tool.Input{fieldPort: in})
		var pr *tool.Problem
		if !errors.As(err, &pr) || !errors.Is(err, errBadPort) {
			t.Errorf("%q: %v", in, err)
		}
	}
}
