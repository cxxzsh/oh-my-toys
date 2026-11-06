package ds

// Component metrics from the design system's components/bundle.css. The
// tokens file names the scale (space-*, radius-*, icon-*); these are the
// few fixed sizes the components set on top of it.
const (
	SideItemHeight     = 30 // .side-item
	SideHeadHeight     = 22 // .side-head
	SidePadX           = 10 // .side padding 0 10px 12px
	SideHeadGlyph      = 12 // .side-head .g
	IconSmGlyph        = 14 // .ti.s .g
	IconMdGlyph        = 18 // .ti.m .g
	IconLgGlyph        = 28 // .ti.l .g
	SearchGlyph        = 15 // .search .g
	KbdHeight          = 18 // .kbd
	PillHeight         = 22 // .pill
	DotSize            = 7  // .dot
	RunDotSize         = 10 // .ti .run
	ChipHeight         = 26 // .chip
	SwitchWidth        = 38 // .sw
	SwitchHeight       = 22
	SwitchKnob         = 18
	CardMinHeight      = 132 // .tc
	DenseMinWidth      = 200 // .grid.dense
	TrayItemHeight     = 32  // .tray-item
	LaunchIcon         = 36  // .launch-item .ti.m
	LaunchIconR        = 9
	LaunchGlyph        = 20
	PaletteWidth       = 600 // .palette
	PaletteInputH      = 52  // .palette-in
	PaletteInputPx     = 17  // .palette-in font-size
	PaletteGlyph       = 20
	PaletteRowH        = 40 // .palette-row
	PaletteMaxRows     = 8  // CommandPalette README: at most 8 results
	PaletteFootH       = 36
	RunRowButtonH      = 24 // .run-row .btn
	ToolRowPadY        = 10 // .tr padding 10px space-3
	MaxPinned          = 8
	TrayMaxLaunch      = 7 // seven pinned + "所有工具"
	SideMinWidth       = 180
	HeaderGap          = 2   // .tool-head name and summary
	ChevronClosed      = -90 // degrees: a closed section's chevron points right
	WindowWidth        = 1100
	WindowHeight       = 720
	WindowMinWidth     = 760
	WindowMinHeight    = 480
	FieldLabelWidth    = 120 // labels of an action tool's fields
	FieldPadX          = 10  // .field padding
	SideSearchGap      = 6   // .search margin-bottom in the sidebar
	SideItemGap        = 2   // .side gap
	SearchPadEnd       = 6   // .search .kbd right
	SearchGap          = 6
	ChipGap            = 6  // .chips gap, .chip gap
	ChipPadX           = 10 // .chip padding
	ChipCountAlpha     = 0.7
	OpenGap            = 2  // .tc-foot .open gap
	SmallGlyph         = 12 // .g.sm
	TrayIconSize       = 16 // points of a menu bar / notification area icon
	TrayIconScale      = 2
	TrayGap            = 6   // between the icon and the panel
	TrayFirstHeight    = 420 // before the panel measures itself
	TrayInset          = 6   // .tray-head, .tray-item padding
	TrayFootPad        = 2   // .tray-foot
	TrayColumns        = 4   // .launch grid
	TrayCellGap        = 2
	LaunchPadTop       = 10 // .launch-item padding 10px 2px 8px
	LaunchGap          = 6
	LaunchLabelLines   = 2  // English names are longer than Chinese ones
	RunRowButtonPadX   = 10 // .run-row .btn
	PaletteFirstHeight = 200
	PaletteTopFraction = 4  // the palette shows a quarter of the way down
	PaletteGap         = 10 // .palette-in gap, .palette-row padding
	PaletteListPad     = 6  // .palette-list
)
