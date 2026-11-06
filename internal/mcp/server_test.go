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

func startSession(ctx context.Context, t *testing.T) *mcpsdk.ClientSession {
	t.Helper()
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
