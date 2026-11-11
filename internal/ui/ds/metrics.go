package ds

import "time"

// Component metrics that the tokens file does not name: the sizes the
// component specs (design/components/*/README.md) and their reference
// drawing set on top of the scale. Everything the tokens do name (control
// heights, rows, windows, glyphs) is in tokens_gen.go.
const (
	SwitchWidth       = 34 // Switch: 34×19
	SwitchHeight      = 19
	SwitchCompactW    = 30 // compact: 30×18, in the tray panel and palette
	SwitchCompactH    = 18
	SwitchInset       = 2 // between the track and the knob
	CheckboxSize      = 15
	CheckTickSize     = 10
	DotLarge          = 8  // StatusDot, large
	RunBadge          = 10 // ToolIcon running badge; RunBadgeSmall on 20pt
	RunBadgeSmall     = 6
	RunBadgeRing      = 2
	BadgePadY         = 1
	CategoryHeight    = 22 // CategoryBadge
	CategoryTracking  = 0.06
	ProgressHeight    = 5
	BusyLine          = 3  // indeterminate strip on the window's top edge
	CountdownSize     = 32 // the countdown of a switch tool, in a mono face
	CountdownLine     = 38
	FactsMinWidth     = 220 // the facts beside the countdown wrap under it below this
	PresetBasis       = 140 // a preset tile's width before the row wraps: several a row
	PresetHeight      = 36
	CustomFieldWidth  = 96  // hours and minutes of a custom run
	ClockFieldWidth   = 128 // a time of day
	SpanFieldWidth    = 84  // a time in a span of a plan
	DayToggleWidth    = 36  // a day of the week in a span
	WeekLabelWidth    = 30  // a day's name beside its bar in the week strip
	WeekBarHeight     = 8   // a day's bar in the week strip
	NowMarkWidth      = 2   // the mark of the time now on a day's bar
	StripHeight       = 10  // PaletteStrip
	SearchGlyph       = 16  // SearchField glyph
	SelectGlyph       = 16  // Select's unfold glyph
	SelectMinWidth    = 128
	SearchMinWidth    = 160
	PaletteGlyph      = 20
	PaletteInputRow   = 48
	PaletteFootRow    = 32
	PaletteMaxRows    = 8
	PaletteTopShare   = 0.2 // the palette's top edge, as a share of the window
	TrayMaxShare      = 0.7 // the tray panel's height, as a share of the screen
	TrayGap           = 6   // between the tray icon and the panel
	TrayIconTopBar    = 18  // tray glyph in a menu bar, points
	TrayIconTaskbar   = 16  // tray glyph in a taskbar, points
	TrayIconScale     = 2   // pixels per point of the tray glyph's image
	TrayHeaderIcon    = 22
	TraySubInset      = 40 // options under a tray switch row, from the panel edge
	DialogMaxItems    = 6
	DaemonPad         = 12
	CardMinColumn     = 256 // ToolCard grid column
	CardMinHeight     = 132
	GlyphSmall        = 14 // glyph on a 22pt ToolIcon
	GlyphMedium       = 17 // on a 28pt
	GlyphLarge        = 22 // on a 38pt
	TitlebarGlyph     = 18 // app icon in the titlebar
	ResultFootPad     = 10
	TableCellPad      = 12
	ProcessGap        = 6
	TooltipGap        = 4   // distance below the control
	TooltipPadY       = 4   // Tooltip padding, vertical (space-4)
	ScrimAlphaOnly    = 1   // marks the scrim as drawn full-strength
	MenuMaxRecent     = 8   // tools in the palette with an empty query
	SidebarMaxJump    = 9   // Mod+1…9 select the first nine tools
	TrafficLightY     = 17  // centers 14pt macOS window buttons in the 48pt titlebar
	SidePeekTrigger   = 14  // left-edge hover strip that opens the unpinned sidebar
	SideHeadHeight    = 24  // collapsible section header in the sidebar
	ChevronClosed     = -90 // degrees: a closed section's chevron points right
	HeaderMetaGap     = 2   // between a page title and its description
	PageMaxWidth      = 0   // pages fill the workbench; there is no cap
	LineWidth         = 1   // every hairline and border
	FocusRingWidth    = 2   // keyboard focus ring and selection borders
	FocusRingOffset   = 2
	FieldHaloWidth    = 3 // TextField focus halo
	TitleTracking     = 0.025
	TextHintDots      = 1 // marks plain hint text in footers
	PercentFull       = 100
	CaretWidth        = 1
	AppIconRadiusDiv  = 4         // the app icon tile's corner, as a fraction of its size
	CodeMaxBytes      = 256 << 10 // CodeView lays out this much of a text, no more
	HexDumpBytes      = 4 << 10   // hex dump of a binary body
	HexRow            = 16        // bytes a hex dump row shows
	HexGroup          = 8         // bytes before the gap in a hex dump row
	TrafficPaneMinH   = 200       // the least height a request or response pane takes when the two stack
	TrafficRulesList  = 360       // the rule list's width beside its form
	TrafficWide       = 1200      // pane width from which the list and detail sit side by side; below it the detail goes under the list
	TrafficDetailWide = 660       // detail pane width from which a flow's request and response sit side by side; below it they stack

	TrafficSourcesWidth = 184 // the sources column beside the flow list
	TrafficSourcesMin   = 920 // page width from which the sources column opens by itself

	SelectionBarW   = 3   // accent bar on selected navigation and table rows
	InspectorWidth  = 272 // side detail drawer width on wide action tables
	PortsWide       = 820 // workbench width from which Ports shows the side inspector
	CLIMaxWidth     = 320 // maximum width of the header CLI parity pill
	SidebarCompactW = 56  // icon-only compact rail width
)

// Widths of the flow list's fixed columns, in pixels, and the least the URL
// column keeps. The URL takes what is left after them.
const (
	TrafficColID       = 50
	TrafficColMethod   = 70
	TrafficColStatus   = 84
	TrafficColProgram  = 110
	TrafficColTime     = 90
	TrafficColDuration = 80
	TrafficColSize     = 90
	TrafficColProtocol = 72
	TrafficColLength   = 88
	TrafficURLMin      = 200
)

// Motion, from the design system's Motion section.
const (
	FadeDuration = 120 * time.Millisecond // tray panel, palette, tooltip fade
	EaseDuration = 150 * time.Millisecond // colour, border and knob changes
	TooltipDelay = 600 * time.Millisecond
)
