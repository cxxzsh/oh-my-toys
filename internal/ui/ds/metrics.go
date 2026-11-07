package ds

import "time"

// Component metrics that the tokens file does not name: the sizes the
// component specs (design/components/*/README.md) and their reference
// drawing set on top of the scale. Everything the tokens do name (control
// heights, rows, windows, glyphs) is in tokens_gen.go.
const (
	SwitchWidth      = 38 // Switch: 38×22
	SwitchHeight     = 22
	SwitchCompactW   = 30 // compact: 30×18, in the tray panel and palette
	SwitchCompactH   = 18
	SwitchInset      = 2 // between the track and the knob
	CheckboxSize     = 14
	CheckTickSize    = 10
	DotLarge         = 8  // StatusDot, large
	RunBadge         = 10 // ToolIcon running badge; RunBadgeSmall on 20pt
	RunBadgeSmall    = 6
	RunBadgeRing     = 2
	BadgePadY        = 1
	CategoryHeight   = 22 // CategoryBadge
	CategoryTracking = 0.06
	ProgressHeight   = 6
	BusyLine         = 3  // indeterminate strip on the window's top edge
	CountdownSize    = 32 // the countdown of a switch tool, in a mono face
	CountdownLine    = 38
	FactsMinWidth    = 220 // the facts beside the countdown wrap under it below this
	PresetBasis      = 280 // a preset tile's width before the row wraps: two a row, as the design shows
	PresetHeight     = 44
	StripHeight      = 10 // PaletteStrip
	SearchGlyph      = 16 // SearchField glyph
	SelectGlyph      = 16 // Select's unfold glyph
	SelectMinWidth   = 128
	SearchMinWidth   = 160
	PaletteGlyph     = 20
	PaletteInputRow  = 48
	PaletteFootRow   = 32
	PaletteMaxRows   = 8
	PaletteTopShare  = 0.2 // the palette's top edge, as a share of the window
	TrayMaxShare     = 0.7 // the tray panel's height, as a share of the screen
	TrayGap          = 6   // between the tray icon and the panel
	TrayIconTopBar   = 18  // tray glyph in a menu bar, points
	TrayIconTaskbar  = 16  // tray glyph in a taskbar, points
	TrayIconScale    = 2   // pixels per point of the tray glyph's image
	TrayHeaderIcon   = 22
	TraySubInset     = 40 // options under a tray switch row, from the panel edge
	DialogMaxItems   = 6
	DaemonPad        = 12
	CardMinColumn    = 270 // ToolCard grid column
	CardMinHeight    = 144
	GlyphSmall       = 15 // glyph on a 24pt ToolIcon
	GlyphMedium      = 20 // on a 32pt
	GlyphLarge       = 32 // on a 52pt
	TitlebarGlyph    = 20 // app icon in the titlebar
	ResultFootPad    = 10
	TableCellPad     = 12
	ProcessGap       = 6
	TooltipGap       = 4   // distance below the control
	TooltipPadY      = 4   // Tooltip padding, vertical (space-4)
	ScrimAlphaOnly   = 1   // marks the scrim as drawn full-strength
	MenuMaxRecent    = 8   // tools in the palette with an empty query
	SidebarMaxJump   = 9   // Mod+1…9 select the first nine tools
	TrafficLightY    = 17  // centers 14pt macOS window buttons in the 48pt titlebar
	SidePeekTrigger  = 14  // left-edge hover strip that opens the unpinned sidebar
	SideHeadHeight   = 24  // collapsible section header in the sidebar
	ChevronClosed    = -90 // degrees: a closed section's chevron points right
	HeaderMetaGap    = 2   // between a page title and its description
	StatusDotPing    = 2   // the ping grows the dot to this scale
	PageMaxWidth     = 0   // pages fill the workbench; there is no cap
	LineWidth        = 1   // every hairline and border
	FocusRingWidth   = 2   // keyboard focus ring and selection borders
	FocusRingOffset  = 2
	FieldHaloWidth   = 3 // TextField focus halo
	TitleTracking    = 0.025
	TextHintDots     = 1 // marks plain hint text in footers
	PercentFull      = 100
	CaretWidth       = 1
	AppIconRadiusDiv = 4 // the app icon tile's corner, as a fraction of its size
)

// Motion, from the design system's Motion section.
const (
	FadeDuration = 120 * time.Millisecond // tray panel, palette, tooltip fade
	EaseDuration = 150 * time.Millisecond // colour, border and knob changes
	PingDuration = time.Second            // live dots
	TooltipDelay = 600 * time.Millisecond
)
