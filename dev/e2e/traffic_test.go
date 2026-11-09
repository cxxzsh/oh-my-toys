package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPollUntilStopsWhenTrue(t *testing.T) {
	calls := 0
	_, err := pollUntil(time.Second, time.Millisecond, func() (bool, error) {
		calls++
		return calls == 3, nil
	})
	if err != nil || calls != 3 {
		t.Fatalf("err=%v calls=%d, want nil after 3 calls", err, calls)
	}
}

func TestPollUntilTimesOut(t *testing.T) {
	_, err := pollUntil(5*time.Millisecond, time.Millisecond, func() (bool, error) { return false, nil })
	if !errors.Is(err, errTimeout) {
		t.Fatalf("err=%v, want errTimeout", err)
	}
}

func TestPollUntilReturnsCheckError(t *testing.T) {
	boom := errors.New("boom")
	_, err := pollUntil(time.Second, time.Millisecond, func() (bool, error) { return false, boom })
	if !errors.Is(err, boom) {
		t.Fatalf("err=%v, want boom", err)
	}
}

func TestPrepareTrafficWritesStartSettings(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "omt-e2e")
	if err := os.WriteFile(exe, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := prepareTraffic(exe); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(exe)
	if _, err := os.Stat(filepath.Join(dir, "portable")); err != nil {
		t.Fatalf("portable marker: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "data", "traffic", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got["ProxyPort"] != float64(19099) || got["SystemProxy"] != false || got["Decrypt"] != false || got["Mode"] != float64(0) {
		t.Fatalf("config = %v", got)
	}
}

func TestMatchFlowFindsRequestRow(t *testing.T) {
	nodes := []node{
		{Role: roleButton, Label: "curl"},
		{Role: roleButton, Label: "#1 GET 127.0.0.1:4000/hello 200 JSON"},
	}
	if !matchFlow(nodes, "127.0.0.1:4000/hello") {
		t.Fatal("flow row not matched")
	}
	if matchFlow(nodes, "127.0.0.1:4001/hello") {
		t.Fatal("other port matched")
	}
	if matchFlow([]node{{Role: roleButton, Label: "curl 127.0.0.1:4000/hello"}}, "127.0.0.1:4000/hello") {
		t.Fatal("sidebar row without # matched as flow")
	}
}

func TestProgramRowIsCaseInsensitive(t *testing.T) {
	n, ok := programRow([]node{{ID: "7", Role: roleButton, Label: "CURL"}})
	if !ok || n.ID != "7" {
		t.Fatalf("programRow = %+v, %v", n, ok)
	}
	if _, ok := programRow([]node{{Role: roleButton, Label: "#1 curl"}}); ok {
		t.Fatal("flow row taken for program row")
	}
}

func TestMissingAllNamesAbsentWants(t *testing.T) {
	nodes := []node{{Role: roleButton, Label: "Start"}}
	missing := missingAll(nodes, []want{
		{"record", roleButton, []string{"Start"}},
		{"search", roleField, []string{"Search"}},
	})
	if len(missing) != 1 || missing[0] != "search" {
		t.Fatalf("missing = %v", missing)
	}
}

func TestSettingsXReadsBounds(t *testing.T) {
	nodes := []node{{Role: roleButton, Label: "Traffic Settings", Bounds: &rect{X: 640, Y: 8}}}
	x, err := settingsX(nodes)
	if err != nil || x != 640 {
		t.Fatalf("x=%v err=%v", x, err)
	}
	if _, err := settingsX(nil); err == nil {
		t.Fatal("missing button not reported")
	}
}

func TestErrorTitlesAreNonEmpty(t *testing.T) {
	for _, title := range errorTitles() {
		if title == "" {
			t.Fatal("empty error title")
		}
	}
}
