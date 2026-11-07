package mcp

import (
	"context"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/cxxzsh/oh-my-toys/internal/core"
	"github.com/cxxzsh/oh-my-toys/internal/tools/awake"
	"github.com/cxxzsh/oh-my-toys/internal/tools/color"
	"github.com/cxxzsh/oh-my-toys/internal/tools/rename"
)

func TestMCPColorAndHiddenApply(t *testing.T) {
	ctx := context.Background()
	session := startSession(ctx, t)
	t.Cleanup(func() { _ = session.Close() })

	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tl := range listed.Tools {
		names[tl.Name] = true
		if tl.Annotations != nil && tl.Annotations.OpenWorldHint != nil && *tl.Annotations.OpenWorldHint {
			t.Errorf("%s open world", tl.Name)
		}
	}
	if !names["color_plan"] || !names["awake_status"] {
		t.Fatalf("tools %v", names)
	}
	if names["rename_apply"] || names["ports_apply"] || names["dupes_apply"] {
		t.Fatal("destructive apply was exposed")
	}

	res, err := session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "color_plan",
		Arguments: map[string]any{"color": "#fff"},
	})
	if err != nil || res.IsError {
		t.Fatal(err, res)
	}
	doc, _ := res.StructuredContent.(map[string]any)
	if doc["summary"] != "#ffffff" {
		t.Fatalf("color %+v", res.StructuredContent)
	}
}

func TestMCPAwakeWithoutApp(t *testing.T) {
	ctx := context.Background()
	session := startSession(ctx, t)
	t.Cleanup(func() { _ = session.Close() })
	res, err := session.CallTool(ctx, &mcpsdk.CallToolParams{Name: "awake_status"})
	if err != nil || res.IsError {
		t.Fatal(err, res)
	}
	doc, _ := res.StructuredContent.(map[string]any)
	if doc["on"] != false {
		t.Fatalf("status %+v", doc)
	}
}

// noApp points the places a command looks for the running desktop app at an
// empty directory, so a test never reaches an app the person has open.
func noApp(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	for _, env := range []string{"HOME", "XDG_CACHE_HOME", "LocalAppData"} {
		t.Setenv(env, dir)
	}
}

func startSession(ctx context.Context, t *testing.T) *mcpsdk.ClientSession {
	t.Helper()
	noApp(t)
	reg, err := core.NewRegistry(awake.New(func() {}), rename.New(), color.New())
	if err != nil {
		t.Fatal(err)
	}
	serverTransport, clientTransport := mcpsdk.NewInMemoryTransports()
	srv := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "oh-my-toys", Version: version}, nil)
	s := Server{Registry: reg}
	for _, tl := range reg.All() {
		s.add(srv, tl)
	}
	go func() { _ = srv.Run(ctx, serverTransport) }()
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: version}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	return session
}

// TestMCPAwakeStartTakesALength: a timed tool's start lists "for" and
// "until" as inputs, and a bad one is refused before anything starts.
func TestMCPAwakeStartTakesALength(t *testing.T) {
	ctx := context.Background()
	session := startSession(ctx, t)
	t.Cleanup(func() { _ = session.Close() })

	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tl := range listed.Tools {
		if tl.Name != "awake_start" {
			continue
		}
		schema, _ := tl.InputSchema.(map[string]any)
		props, _ := schema["properties"].(map[string]any)
		if props["for"] == nil || props["until"] == nil {
			t.Fatalf("awake_start inputs %v, want for and until", schema)
		}
	}
	res, err := session.CallTool(ctx, &mcpsdk.CallToolParams{Name: "awake_start", Arguments: map[string]any{"until": "25:00"}})
	if err != nil || !res.IsError {
		t.Fatalf("a bad time of day: %v, %+v", err, res)
	}
}
