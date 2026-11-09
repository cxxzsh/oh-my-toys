package shell

import (
	"testing"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/tools/traffic/flowlog"
)

// TestTrafficDetailFollowsFlowToCompletion selects a flow that is still open
// in the real log, then completes it there, and checks the detail shows the
// response without the row being re-selected.
func TestTrafficDetailFollowsFlowToCompletion(t *testing.T) {
	log := flowlog.New(100, 1<<20)
	f := testGET("api.example.com", "/get", 0)
	f.State, f.Status, f.Info = tool.FlowOpen, 0, ""
	f.ID = log.Put(f)

	src := newTestTraffic()
	src.live = log
	tt := openTraffic(t, src)
	waitText(t, tt, i18n.TrafficOpen())

	f.State, f.Status, f.Info = tool.FlowDone, 200, "OK"
	log.Put(f)
	waitText(t, tt, "200 OK")
}
