package ui

import "image/color"

// ── Surface hierarchy (3 distinct background levels + raised) ────────────
var (
	// Level 0 (base): deepest background — grid pane, timeline
	colBGTop    = color.RGBA{12, 13, 16, 255}
	colBGBottom = color.RGBA{12, 13, 16, 255}

	// Level 1 (surface): main surfaces — row rack, transport bg
	colSurface1 = color.RGBA{20, 21, 26, 255}

	// Level 2 (elevated): cards, buttons, input fields
	colSurface2 = color.RGBA{28, 30, 36, 255}

	// Level 3 (raised): hover states, active surfaces
	colSurface3 = color.RGBA{36, 38, 46, 255}

	// ── Grid colors (brightened slightly for visibility against Level 0) ──
	colGridLine         = color.RGBA{38, 40, 48, 255}
	colGridHalf         = color.RGBA{46, 48, 56, 255}
	colGridQuarter      = color.RGBA{54, 56, 64, 255}
	colGridEighth       = color.RGBA{62, 64, 72, 255}
	colGridSixteenth    = color.RGBA{70, 72, 80, 255}
	colGridThirtySecond = color.RGBA{78, 80, 88, 255}

	// ── Borders (structured hierarchy) ──────────────────────────────────
	colBorderSubtle = color.NRGBA{255, 255, 255, 8}  // section dividers
	colBorderMedium = color.NRGBA{255, 255, 255, 15} // control borders
	colBorderStrong = color.NRGBA{255, 255, 255, 25} // focused input borders

	colButtonBorder = color.NRGBA{255, 255, 255, 15} // alias for control borders
	colSubtleBorder = color.NRGBA{255, 255, 255, 8}  // alias for subtle borders

	// ── Accent (Primary — Cyan) ─────────────────────────────────────────
	colAccent      = color.RGBA{0, 200, 255, 255}    // default cyan
	colAccentBright = color.RGBA{80, 220, 255, 255}  // hover/active
	colAccentDim   = color.RGBA{0, 140, 200, 255}    // splitter, secondary
	colAccentSubtle = color.NRGBA{0, 200, 255, 20}   // backgrounds/fills

	// ── Accent (Secondary — Semantic) ───────────────────────────────────
	colPlayGreen = color.RGBA{60, 210, 120, 255} // play state
	colStopRed   = color.RGBA{220, 70, 70, 255} // stop/delete
	colMuteRed   = color.RGBA{160, 60, 50, 255} // mute state

	// ── Text colors ─────────────────────────────────────────────────────
	colTextPrimary   = color.RGBA{220, 222, 228, 255} // main text
	colTextSecondary = color.RGBA{140, 142, 150, 255} // labels, captions
	colTextDisabled  = color.RGBA{80, 82, 90, 255}    // inactive text
	colTextAccent    = color.RGBA{0, 200, 255, 255}    // links, active labels

	// ── Legacy aliases (mapped to new surface system) ───────────────────
	colPlayButton = color.RGBA{36, 160, 90, 255}  // slightly richer green
	colStopButton = color.RGBA{180, 55, 55, 255}  // slightly richer red
	colBPMBox     = colSurface2                     // elevated surface for inputs
	colDropdown   = colSurface2                     // elevated surface for dropdowns
	colDropdownEdge = color.NRGBA{0, 200, 255, 50} // cyan accent border
	colError      = color.RGBA{220, 60, 60, 255}

	// Secondary button fill for file-ops, overflow, etc.
	colSecondaryBtn = colSurface2

	// Row control button inactive state
	colRowBtnInactive     = colSurface1
	colRowBtnInactiveText = colTextSecondary

	// Row label brighter color
	colRowLabel = colTextPrimary

	// Splitter handle colors (pill indicator on divider lines)
	colSplitterHandle        = color.NRGBA{200, 200, 210, 220}
	colSplitterHandleMobile  = color.NRGBA{0, 140, 200, 180} // accent dim for visibility
	colSplitterHandleHover   = color.NRGBA{255, 255, 255, 240}
	colSplitterGripLine      = color.NRGBA{100, 100, 110, 180}
	colSplitterGripLineHover = color.NRGBA{180, 180, 190, 200}

	// Rack surface: Level 1 for visual separation
	colRackSurface = colSurface1

	// Panel/popup surface colors — unified component kit
	colPanelBG     = color.RGBA{24, 25, 32, 250}    // between L0 and L1
	colPanelBorder = color.NRGBA{255, 255, 255, 20} // medium border
	colScrim       = color.NRGBA{0, 0, 0, 150}      // backdrop

	// Delete/destructive action colors
	colDeleteFill   = color.RGBA{120, 35, 35, 255}
	colDeleteBorder = color.RGBA{180, 55, 55, 255}
	colDeleteText   = colStopRed // red text for delete items (less aggressive than red bg)

	// Context menu group container backgrounds
	colMenuGroupBG       = color.NRGBA{255, 255, 255, 8}  // group container bg
	colMenuGroupDeleteBG = color.NRGBA{220, 50, 50, 15}   // delete group tint
	colMenuIcon          = colTextDisabled                  // icon tint

	colStep          = colAccent                        // cyan cells
	colStepOff       = color.RGBA{14, 15, 18, 255}     // just above L0 for depth
	colStepBorder    = color.NRGBA{255, 255, 255, 10}   // very subtle cell borders
	colHighlight     = color.RGBA{240, 242, 248, 255}   // cool white flash
	colMuteCell      = color.RGBA{100, 100, 110, 255}
	colMuteHighlight = color.RGBA{180, 182, 190, 160} // brighter gray for white highlight

	// Beat grouping: alternate column tint for visual beat separation
	colBeatGroupAlt = color.NRGBA{255, 255, 255, 4}

	colTimelineTotal        = color.RGBA{16, 17, 22, 255}   // darker for timeline bg
	colTimelineView         = color.NRGBA{0, 200, 255, 60}
	colTimelineViewHi       = color.RGBA{0, 220, 255, 255}
	colTimelineCursor       = color.RGBA{80, 220, 255, 255} // bright cyan (was yellow)
	colTimelineBeat         = color.RGBA{80, 82, 90, 255}
	colEQBg                 = color.RGBA{14, 16, 22, 220}
	colEQBar                = color.RGBA{40, 220, 240, 255}
	colEQBarPeak            = color.RGBA{240, 120, 80, 255}
	colWaveTrace            = color.RGBA{120, 220, 255, 255}
	colWaveTraceDry         = color.RGBA{100, 140, 180, 100}
	colWaveMid              = color.RGBA{90, 120, 140, 200}
	colEQCurve              = color.RGBA{0, 200, 255, 230}
	colEQCurveFill          = color.NRGBA{0, 200, 255, 20}
	colEQHandle             = colAccent
	colEQHandleActive       = colAccentBright
	colEQHandleBorder       = colAccentDim
	colEQZeroLine           = color.RGBA{120, 120, 130, 80}
	colEQFilterHandle       = color.RGBA{80, 220, 220, 255}
	colEQFilterHandleActive = color.RGBA{140, 255, 255, 255}
	colEQFilterLine         = color.RGBA{80, 220, 220, 60}

	NodeUI   = NodeStyle{Radius: 16, Fill: color.RGBA{40, 42, 52, 255}, Border: color.RGBA{140, 142, 155, 255}}
	SignalUI = SignalStyle{Radius: 6, Color: colAccent}
	EdgeUI   = EdgeStyle{Color: color.RGBA{120, 122, 140, 100}}

	// Unified increment/decrement button color
	colIncDec       = color.RGBA{30, 42, 68, 255}
	colIncDecBorder = color.RGBA{70, 130, 210, 255}

	// Transport surface: Level 1 for visual hierarchy
	colTransportSurface = colSurface1
	// Transport border: subtle section divider
	colTransportBorder = colBorderSubtle
	// Transport container border (visible grouping)
	colTransportContainerBorder = colBorderMedium
	// Transport group divider
	colTransportDivider = color.NRGBA{255, 255, 255, 20}

	// Pill container colors for transport button groups
	colTransportGroupBG     = color.NRGBA{255, 255, 255, 10} // pill container fill
	colTransportGroupBorder = color.NRGBA{255, 255, 255, 15} // pill container border

	// Icon tint colors for mobile transport buttons
	colPlayIconTint  = colPlayGreen                    // green for play icon
	colStopIconTint  = colStopRed                      // red for stop icon
	colIncDecIcon    = colTextPrimary                   // primary text for +/- icons (mobile)
	colIncDecIconHi  = color.RGBA{106, 176, 240, 255}  // blue for +/- icons (desktop)
	colVolumeIconOn  = colTextPrimary                   // primary when volume > 0
	colVolumeIconOff = colTextDisabled                  // disabled when muted/zero
	colFollowActive  = colAccentBright                  // bright cyan for follow-on
	colRecordIdle    = color.NRGBA{200, 50, 50, 255}   // red circle for record button
	colRecordActive  = color.NRGBA{240, 40, 40, 255}   // bright red when recording

	// Mute/Solo active state colors
	colMuteActive    = colMuteRed
	colMuteActiveBdr = color.RGBA{200, 90, 70, 255}
	colSoloActive    = color.RGBA{80, 220, 255, 255}  // bright cyan (was amber)
	colSoloActiveBdr = color.RGBA{120, 235, 255, 255} // lighter cyan border

	PlayButtonStyle = ButtonStyle{Fill: colPlayButton, Border: color.RGBA{50, 200, 110, 255}}
	StopButtonStyle = ButtonStyle{Fill: colStopButton, Border: color.RGBA{210, 80, 80, 255}}
	BPMBoxStyle     = TextInputStyle{Fill: colSurface2, Border: colBorderMedium, Cursor: color.White}
	// All +/- buttons use the same unified blue style
	BPMDecStyle         = ButtonStyle{Fill: colIncDec, Border: colIncDecBorder}
	BPMIncStyle         = ButtonStyle{Fill: colIncDec, Border: colIncDecBorder}
	LenDecStyle         = ButtonStyle{Fill: colIncDec, Border: colIncDecBorder}
	LenIncStyle         = ButtonStyle{Fill: colIncDec, Border: colIncDecBorder}
	InstButtonStyle     = ButtonStyle{Fill: colSurface2, Border: colBorderSubtle}
	UploadBtnStyle      = ButtonStyle{Fill: colSurface2, Border: colBorderSubtle}
	PopupButtonStyle    = ButtonStyle{Fill: colSurface2, Border: colBorderSubtle}
	DropdownStyle       = ButtonStyle{Fill: colSurface2, Border: colDropdownEdge}
	DisabledButtonStyle = ButtonStyle{Fill: color.RGBA{50, 52, 58, 255}, Border: colBorderSubtle}
	// Delete action style
	DeleteButtonStyle        = ButtonStyle{Fill: colDeleteFill, Border: colDeleteBorder}
	DeleteConfirmButtonStyle = ButtonStyle{Fill: color.RGBA{200, 60, 60, 255}, Border: color.RGBA{255, 80, 80, 255}}
	// Missing instrument row label style
	MissingInstStyle = ButtonStyle{Fill: colError, Border: colBorderMedium}
	// Mute/Solo active styles for drum row buttons
	MuteActiveStyle = ButtonStyle{Fill: colMuteActive, Border: colMuteActiveBdr}
	SoloActiveStyle = ButtonStyle{Fill: colSoloActive, Border: colSoloActiveBdr}
	// FX active style (cyan accent)
	FXActiveStyle = ButtonStyle{Fill: color.RGBA{20, 40, 56, 255}, Border: colAccent}

	// Context menu items: transparent — container provides visual boundary
	ContextMenuItemStyle = ButtonStyle{Fill: color.RGBA{0, 0, 0, 0}, Border: color.NRGBA{0, 0, 0, 0}}

	// EQ band mute button style (red when active, gray when inactive)
	EQMuteButtonStyle       = ButtonStyle{Fill: colSurface3, Border: colBorderMedium}
	EQMuteButtonActiveStyle = ButtonStyle{Fill: color.RGBA{180, 50, 50, 255}, Border: colBorderMedium}
	// EQ filter (HPF/LPF) toggle button style (teal when active)
	EQFilterButtonActiveStyle = ButtonStyle{Fill: color.RGBA{40, 160, 160, 255}, Border: color.RGBA{80, 220, 220, 255}}

	// EQ per-band dB text input style
	EQDBBoxStyle = TextInputStyle{Fill: colSurface2, Border: colBorderMedium, Cursor: color.White}

	colEQDBLabel      = colTextPrimary
	colEQDBLabelMuted = colTextSecondary

	// ── Transport button styles (mobile uses surface2 with subtle borders) ──
	TransportPlayStyle     = ButtonStyle{Fill: colSurface2, Border: colBorderSubtle}
	TransportStopStyle     = ButtonStyle{Fill: colSurface2, Border: colBorderSubtle}
	TransportIncStyle      = ButtonStyle{Fill: colSurface2, Border: colBorderSubtle}
	TransportDecStyle      = ButtonStyle{Fill: colSurface2, Border: colBorderSubtle}
	TransportMiscStyle     = ButtonStyle{Fill: colSurface2, Border: colBorderSubtle}
	TransportFollowOnStyle = ButtonStyle{Fill: color.RGBA{24, 48, 72, 255}, Border: color.NRGBA{80, 200, 255, 80}}
	FABStyle               = ButtonStyle{Fill: colAccent, Border: color.NRGBA{255, 255, 255, 30}}
	MobileRowLabelStyle    = ButtonStyle{Fill: colSurface1, Border: colBorderSubtle}

	DrumCellUI = DrumCellStyle{
		On:        colStep,
		Off:       colStepOff,
		Highlight: colHighlight,
		Border:    colStepBorder,
	}

	// instColors maps instrument IDs to their display colors.
	// Coordinated palette with good contrast against dark surfaces.
	instColors = map[string]color.Color{
		"snare": color.RGBA{195, 72, 80, 255},  // warm red (desaturated ~20%)
		"kick":  color.RGBA{50, 170, 100, 255}, // green (desaturated ~20%)
		"hihat": color.RGBA{200, 168, 45, 255}, // gold (desaturated ~20%)
		"tom":   color.RGBA{68, 100, 195, 255}, // blue (desaturated ~20%)
		"clap":  color.RGBA{185, 68, 168, 255}, // magenta (desaturated ~20%)
	}

	// palette used for user-loaded instruments or any ids not in instColors.
	customPalette = []color.Color{
		color.RGBA{68, 168, 168, 255}, // cyan (desaturated ~20%)
		color.RGBA{170, 110, 68, 255}, // orange (desaturated ~20%)
		color.RGBA{110, 68, 168, 255}, // purple (desaturated ~20%)
		color.RGBA{170, 68, 110, 255}, // pink (desaturated ~20%)
		color.RGBA{68, 168, 110, 255}, // spring green (desaturated ~20%)
	}

	customColors    = map[string]color.Color{}
	nextCustomColor int
)
