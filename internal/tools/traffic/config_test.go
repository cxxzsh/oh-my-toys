package traffic

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

func TestConfigDefaultsAreSavedAndRead(t *testing.T) {
	tl, _ := testTool(t)
	if got := tl.Config(); !sameConfig(got, defaultConfig()) {
		t.Fatalf("Config() = %+v, want the defaults", got)
	}
	want := tool.CaptureConfig{
		Mode:        tool.ModePackets,
		ProxyPort:   9123,
		SystemProxy: false,
		Decrypt:     false,
		Bypass:      []string{"*.local", "example.test"},
		Interface:   "en0",
		Upstream:    "http://10.0.0.2:3128",
	}
	if err := tl.SetConfig(want); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	path := tl.path(configFile)
	checkMode(t, path, fileMode)
	checkMode(t, tl.dir, folderMode)

	again := newTool(filepath.Dir(tl.dir), (&fakeSystem{}).system())
	if got := again.Config(); !sameConfig(got, want) {
		t.Errorf("Config() after a restart = %+v, want %+v", got, want)
	}
}

func TestConfigFileDamageFallsBackToDefaults(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "not JSON", body: "{  nope"},
		{name: "an unusable port", body: `{"Mode":0,"ProxyPort":80,"SystemProxy":true,"Decrypt":true}`},
		{name: "an unknown mode", body: `{"Mode":7,"ProxyPort":9099}`},
		{name: "a bad upstream", body: `{"Mode":0,"ProxyPort":9099,"Upstream":"ftp://10.0.0.2:21"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tl, _ := testTool(t)
			if err := os.MkdirAll(tl.dir, folderMode); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			path := tl.path(configFile)
			if err := os.WriteFile(path, []byte(tt.body), fileMode); err != nil {
				t.Fatalf("write: %v", err)
			}
			if got := newTool(filepath.Dir(tl.dir), (&fakeSystem{}).system()).Config(); !sameConfig(got, defaultConfig()) {
				t.Errorf("Config() = %+v, want the defaults", got)
			}
		})
	}
}

func TestSetConfigValidation(t *testing.T) {
	tests := []struct {
		name  string
		cfg   tool.CaptureConfig
		valid bool
	}{
		{name: "the defaults", cfg: defaultConfig(), valid: true},
		{name: "packet mode", cfg: tool.CaptureConfig{Mode: tool.ModePackets, ProxyPort: 9099}, valid: true},
		{name: "another port", cfg: tool.CaptureConfig{Mode: tool.ModeProxy, ProxyPort: 1024}, valid: true},
		{name: "the top port", cfg: tool.CaptureConfig{Mode: tool.ModeProxy, ProxyPort: 65535}, valid: true},
		{name: "a socks5 upstream", cfg: tool.CaptureConfig{Mode: tool.ModeProxy, ProxyPort: 9099, Upstream: "socks5://10.0.0.2:1080"}, valid: true},
		{name: "port zero", cfg: tool.CaptureConfig{Mode: tool.ModeProxy, ProxyPort: 0}},
		{name: "a privileged port", cfg: tool.CaptureConfig{Mode: tool.ModeProxy, ProxyPort: 80}},
		{name: "a port that is not a port", cfg: tool.CaptureConfig{Mode: tool.ModeProxy, ProxyPort: 70000}},
		{name: "an unknown mode", cfg: tool.CaptureConfig{Mode: tool.CaptureMode(9), ProxyPort: 9099}},
		{name: "an upstream without a scheme", cfg: tool.CaptureConfig{Mode: tool.ModeProxy, ProxyPort: 9099, Upstream: "10.0.0.2:3128"}},
		{name: "an upstream of another scheme", cfg: tool.CaptureConfig{Mode: tool.ModeProxy, ProxyPort: 9099, Upstream: "ftp://10.0.0.2:21"}},
		{name: "an upstream without a port", cfg: tool.CaptureConfig{Mode: tool.ModeProxy, ProxyPort: 9099, Upstream: "http://10.0.0.2"}},
		{name: "an empty bypass entry", cfg: tool.CaptureConfig{Mode: tool.ModeProxy, ProxyPort: 9099, Bypass: []string{"*.local", " "}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { checkSetConfig(t, tt.cfg, tt.valid) })
	}
}

// checkSetConfig sets the settings and checks that a bad one changes
// nothing.
func checkSetConfig(t *testing.T, cfg tool.CaptureConfig, valid bool) {
	t.Helper()
	tl, _ := testTool(t)
	err := tl.SetConfig(cfg)
	if valid {
		if err != nil {
			t.Fatalf("SetConfig: %v", err)
		}
		return
	}
	if !errors.Is(err, ErrBadConfig) {
		t.Errorf("SetConfig: %v, want ErrBadConfig", err)
	}
	if got := tl.Config(); !sameConfig(got, defaultConfig()) {
		t.Errorf("Config() = %+v, want the settings unchanged", got)
	}
	if _, statErr := os.Stat(tl.path(configFile)); !errors.Is(statErr, fs.ErrNotExist) {
		t.Errorf("stat %s: %v, want no file written", configFile, statErr)
	}
}

func TestSetConfigRestartsTheCaptureInProgress(t *testing.T) {
	tl, _ := testTool(t)
	first := proxyConfig(t, false)
	if err := tl.SetConfig(first); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if err := tl.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = tl.Stop() })

	second := first
	second.ProxyPort = freePort(t)
	if err := tl.SetConfig(second); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	run, ok := tl.runningProxy()
	if !ok {
		t.Fatal("the capture stopped instead of restarting")
	}
	if want := fmt.Sprintf("127.0.0.1:%d", second.ProxyPort); run.addr != want {
		t.Errorf("capture on %s, want %s", run.addr, want)
	}
}

func TestSetConfigWithoutACaptureStartsNothing(t *testing.T) {
	tl, fake := testTool(t)
	if err := tl.SetConfig(proxyConfig(t, false)); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if st := tl.Status(); st.On {
		t.Errorf("status %+v, want off: settings alone start no capture", st)
	}
	if fake.seenPrefix("enable ") {
		t.Error("the system proxy was changed by settings alone")
	}
}

// TestSetConfigRejectsTheCaptureOwnAddress: an upstream that names this
// very proxy bounces every request back into it.
func TestSetConfigRejectsTheCaptureOwnAddress(t *testing.T) {
	const port = defaultProxyPort
	tests := []string{
		"http://127.0.0.1:%d",
		"http://localhost:%d",
		"socks5://[::1]:%d",
		"http://0.0.0.0:%d",
		"socks5h://127.0.0.1:%d",
	}
	for _, pattern := range tests {
		text := fmt.Sprintf(pattern, port)
		t.Run(text, func(t *testing.T) {
			tl, _ := testTool(t)
			cfg := defaultConfig()
			cfg.Upstream = text
			if err := tl.SetConfig(cfg); !errors.Is(err, ErrBadConfig) {
				t.Errorf("SetConfig with upstream %s = %v, want ErrBadConfig", text, err)
			}
		})
	}

	tl, _ := testTool(t)
	cfg := defaultConfig()
	cfg.Upstream = fmt.Sprintf("http://127.0.0.1:%d", defaultProxyPort+1)
	if err := tl.SetConfig(cfg); err != nil {
		t.Errorf("SetConfig with another loopback port: %v", err)
	}
}

// TestWriteJSONLeavesNoTemporaryFiles: a write either replaces the file or
// leaves the old one, and never leaves a scratch file next to it.
func TestWriteJSONLeavesNoTemporaryFiles(t *testing.T) {
	tl, _ := testTool(t)
	if err := tl.SetConfig(defaultConfig()); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	leftovers, err := filepath.Glob(tl.path("*.tmp"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(leftovers) != 0 {
		t.Errorf("left behind %v", leftovers)
	}
}
