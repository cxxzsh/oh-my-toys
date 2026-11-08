// Package screenshot is the screenshot tool: its manifest, which puts it in
// the sidebar, the palette and its own page. The desktop app draws the
// overlay and does the work, because a capture needs the screen and the
// person in front of it; the tool declares no actions, so the command line
// and MCP leave it alone.
package screenshot

import (
	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// ID is the tool's ID.
const ID = "screenshot"

// Tool is Screenshot.
type Tool struct{}

// New makes the tool.
func New() *Tool { return &Tool{} }

// Manifest describes the tool.
func (*Tool) Manifest() tool.Manifest {
	return tool.Manifest{
		ID:        ID,
		Name:      i18n.Messages.SnapName,
		Summary:   i18n.Messages.SnapSummary,
		Category:  tool.CategoryMedia,
		Kind:      tool.KindCapture,
		Icon:      "screenshot",
		Keywords:  []string{"screenshot", "capture", "screen", "snip", "截图", "截屏", "抓屏"},
		Platforms: []string{"macos"},
	}
}

// Specs declares no actions: only the desktop app runs a capture.
func (*Tool) Specs() []tool.Spec { return nil }
