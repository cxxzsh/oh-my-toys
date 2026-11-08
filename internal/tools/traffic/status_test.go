package traffic

import (
	"context"
	"fmt"
	"testing"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

func TestStatusOff(t *testing.T) {
	tl, _ := testTool(t)
	st := tl.Status()
	if st.On {
		t.Errorf("Status = %+v, want off", st)
	}
	if st.Text != i18n.StatusOff() {
		t.Errorf("text %q, want %q", st.Text, i18n.StatusOff())
	}
	if st.Detail != "" || !st.Since.IsZero() {
		t.Errorf("Status = %+v, want no detail and no start", st)
	}
}

func TestStatusOfAProxyCapture(t *testing.T) {
	tl, fake := testTool(t)
	makeRoot(t, tl)
	cfg := proxyConfig(t, false)
	cfg.Decrypt = true
	if err := tl.SetConfig(cfg); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if err := tl.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = tl.Stop() })

	st := tl.Status()
	want := fmt.Sprintf("127.0.0.1:%d", cfg.ProxyPort)
	if !st.On {
		t.Fatalf("Status = %+v, want on", st)
	}
	if wantText := i18n.TrafficRunningProxy(want, 0); st.Text != wantText {
		t.Errorf("text %q, want %q", st.Text, wantText)
	}
	if st.Detail != i18n.TrafficDecrypt() {
		t.Errorf("detail %q, want %q", st.Detail, i18n.TrafficDecrypt())
	}
	if !st.Since.Equal(fake.now) {
		t.Errorf("since %v, want %v", st.Since, fake.now)
	}
}

func TestStatusOfACaptureThatTunnelsHTTPS(t *testing.T) {
	tl, fake := testTool(t)
	cfg := proxyConfig(t, false)
	cfg.Decrypt = false
	if err := tl.SetConfig(cfg); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if err := tl.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = tl.Stop() })
	if st := tl.Status(); st.Detail != "" {
		t.Errorf("detail %q, want none while HTTPS is tunnelled", st.Detail)
	}
	if fake.seen("trusted") {
		t.Error("the trust store was read while decrypting is off")
	}
}

func TestLogFollowsTheMode(t *testing.T) {
	tl, _ := testTool(t)
	if tl.Log() != tool.FlowLog(tl.proxylog) {
		t.Error("Log() is not the proxy log in proxy mode")
	}
	packets := tool.CaptureConfig{Mode: tool.ModePackets, ProxyPort: defaultProxyPort}
	if err := tl.SetConfig(packets); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if tl.Log() != tool.FlowLog(tl.packetlog) {
		t.Error("Log() is not the packet log in packet mode")
	}
	if err := tl.SetConfig(defaultConfig()); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if tl.Log() != tool.FlowLog(tl.proxylog) {
		t.Error("Log() is not the proxy log after the mode changed back")
	}
}

// TestStatusCountsWhatTheLogHolds: the count is the log's own, without
// rebuilding its rows on every frame.
func TestStatusCountsWhatTheLogHolds(t *testing.T) {
	tl, _ := testTool(t)
	cfg := proxyConfig(t, false)
	if err := tl.SetConfig(cfg); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if err := tl.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = tl.Stop() })
	tl.proxylog.Put(tool.Flow{FlowRow: tool.FlowRow{Source: tool.SourceProxy, Host: "example.test"}})
	st := tl.Status()
	want := i18n.TrafficRunningProxy(fmt.Sprintf("127.0.0.1:%d", cfg.ProxyPort), 1)
	if st.Text != want {
		t.Errorf("text %q, want %q", st.Text, want)
	}
}
