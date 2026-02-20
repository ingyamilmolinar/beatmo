package ui

import "image/color"

var (
	colBGTop            = color.RGBA{24, 24, 30, 255}
	colBGBottom         = color.RGBA{18, 18, 22, 255}
	colGridLine         = color.RGBA{42, 42, 50, 255}
	colGridHalf         = color.RGBA{50, 50, 58, 255}
	colGridQuarter      = color.RGBA{58, 58, 66, 255}
	colGridEighth       = color.RGBA{66, 66, 74, 255}
	colGridSixteenth    = color.RGBA{74, 74, 82, 255}
	colGridThirtySecond = color.RGBA{82, 82, 90, 255}

	colButtonBorder = color.RGBA{120, 120, 130, 255}
	colSubtleBorder = color.RGBA{90, 90, 100, 255}
	colPlayButton   = color.RGBA{40, 200, 100, 255}
	colStopButton   = color.RGBA{220, 60, 60, 255}
	colBPMBox       = color.RGBA{34, 34, 42, 255}
	colDropdown     = color.RGBA{48, 48, 58, 255}
	colDropdownEdge = color.RGBA{255, 220, 40, 255}
	colError        = color.RGBA{200, 60, 60, 255}

	// Splitter handle colors (pill indicator on divider lines)
	colSplitterHandle        = color.NRGBA{200, 200, 210, 220}
	colSplitterHandleMobile  = color.NRGBA{180, 180, 190, 180}
	colSplitterHandleHover   = color.NRGBA{255, 255, 255, 240}
	colSplitterGripLine      = color.NRGBA{100, 100, 110, 180}
	colSplitterGripLineHover = color.NRGBA{180, 180, 190, 200}

	// Rack surface: slightly lighter than colBGBottom to distinguish
	// the controls area from unintended gaps.
	colRackSurface = color.RGBA{22, 22, 28, 255}

	// Panel/popup surface colors — unified component kit
	colPanelBG     = color.RGBA{26, 26, 34, 250}    // popup/menu background (deeper, less transparent)
	colPanelBorder = color.NRGBA{255, 255, 255, 20} // subtle popup border
	colScrim       = color.NRGBA{0, 0, 0, 150}      // heavier backdrop scrim for focus

	// Delete/destructive action colors
	colDeleteFill   = color.RGBA{120, 35, 35, 255}
	colDeleteBorder = color.RGBA{180, 55, 55, 255}

	colStep          = color.RGBA{0, 185, 235, 255}
	colStepOff       = color.RGBA{26, 26, 32, 255}
	colStepBorder    = color.RGBA{50, 50, 58, 255}
	colHighlight     = color.RGBA{255, 220, 40, 255}
	colMuteCell      = color.RGBA{100, 100, 110, 255}
	colMuteHighlight = color.RGBA{90, 90, 100, 160}

	colTimelineTotal        = color.RGBA{28, 28, 36, 255}
	colTimelineView         = color.RGBA{0, 185, 235, 80}
	colTimelineViewHi       = color.RGBA{0, 220, 255, 255}
	colTimelineCursor       = color.RGBA{255, 220, 40, 255}
	colTimelineBeat         = color.RGBA{100, 100, 100, 255}
	colEQBg                 = color.RGBA{15, 20, 26, 220}
	colEQBar                = color.RGBA{40, 220, 240, 255}
	colEQBarPeak            = color.RGBA{240, 120, 80, 255}
	colWaveTrace            = color.RGBA{120, 220, 255, 255}
	colWaveTraceDry         = color.RGBA{100, 140, 180, 100} // dim blue-gray for pre-EQ
	colWaveMid              = color.RGBA{90, 120, 140, 200}
	colEQCurve              = color.RGBA{255, 180, 50, 200}  // warm orange curve line
	colEQCurveFill          = color.RGBA{255, 180, 50, 30}   // subtle fill between curve and 0dB
	colEQHandle             = color.RGBA{255, 200, 80, 255}  // handle dot
	colEQHandleActive       = color.RGBA{255, 240, 140, 255} // handle when dragging
	colEQHandleBorder       = color.RGBA{200, 150, 50, 255}  // handle outline
	colEQZeroLine           = color.RGBA{120, 120, 130, 80}  // 0 dB reference line
	colEQFilterHandle       = color.RGBA{80, 220, 220, 255}  // teal for HPF/LPF handles
	colEQFilterHandleActive = color.RGBA{140, 255, 255, 255} // bright teal when dragging
	colEQFilterLine         = color.RGBA{80, 220, 220, 60}   // vertical cutoff reference line

	NodeUI   = NodeStyle{Radius: 16, Fill: color.RGBA{50, 50, 62, 255}, Border: color.RGBA{150, 150, 165, 255}}
	SignalUI = SignalStyle{Radius: 6, Color: color.RGBA{0, 210, 255, 255}}
	EdgeUI   = EdgeStyle{Color: color.RGBA{130, 130, 150, 100}}

	// Unified increment/decrement button color
	colIncDec       = color.RGBA{60, 120, 200, 255}
	colIncDecBorder = color.RGBA{90, 150, 230, 255}

	// Transport surface: slightly elevated from colBGBottom for visual hierarchy
	colTransportSurface = color.RGBA{34, 34, 42, 255}
	// Transport border: subtler than colDropdownEdge
	colTransportBorder = color.NRGBA{255, 255, 255, 18}

	// Icon tint colors for mobile transport buttons
	colPlayIconTint  = color.RGBA{90, 230, 130, 255}  // bright green for play icon
	colStopIconTint  = color.RGBA{230, 95, 95, 255}   // bright red for stop icon
	colIncDecIcon    = color.RGBA{190, 190, 200, 255} // bright gray for +/- icons (mobile)
	colIncDecIconHi  = color.RGBA{240, 240, 255, 255} // high-contrast white for +/- icons (desktop)
	colVolumeIconOn  = color.RGBA{180, 180, 190, 255} // light gray when volume > 0
	colVolumeIconOff = color.RGBA{80, 80, 90, 255}    // dim gray when muted/zero
	colFollowActive  = color.RGBA{100, 180, 255, 255} // bright blue for follow-on

	// Mute/Solo active state colors
	colMuteActive    = color.RGBA{180, 70, 50, 255}
	colMuteActiveBdr = color.RGBA{220, 100, 80, 255}
	colSoloActive    = color.RGBA{200, 170, 40, 255}
	colSoloActiveBdr = color.RGBA{240, 210, 60, 255}

	PlayButtonStyle = ButtonStyle{Fill: colPlayButton, Border: color.RGBA{80, 240, 140, 255}}
	StopButtonStyle = ButtonStyle{Fill: colStopButton, Border: color.RGBA{255, 100, 100, 255}}
	BPMBoxStyle     = TextInputStyle{Fill: colBPMBox, Border: color.RGBA{80, 80, 90, 255}, Cursor: color.White}
	// All +/- buttons use the same unified blue style
	BPMDecStyle         = ButtonStyle{Fill: colIncDec, Border: colIncDecBorder}
	BPMIncStyle         = ButtonStyle{Fill: colIncDec, Border: colIncDecBorder}
	LenDecStyle         = ButtonStyle{Fill: colIncDec, Border: colIncDecBorder}
	LenIncStyle         = ButtonStyle{Fill: colIncDec, Border: colIncDecBorder}
	InstButtonStyle     = ButtonStyle{Fill: colBPMBox, Border: colSubtleBorder}
	UploadBtnStyle      = ButtonStyle{Fill: colBPMBox, Border: colSubtleBorder}
	PopupButtonStyle    = ButtonStyle{Fill: colBPMBox, Border: colSubtleBorder}
	DropdownStyle       = ButtonStyle{Fill: colDropdown, Border: colDropdownEdge}
	DisabledButtonStyle = ButtonStyle{Fill: color.RGBA{70, 70, 70, 255}, Border: colButtonBorder}
	// Delete action style
	DeleteButtonStyle        = ButtonStyle{Fill: colDeleteFill, Border: colDeleteBorder}
	DeleteConfirmButtonStyle = ButtonStyle{Fill: color.RGBA{200, 60, 60, 255}, Border: color.RGBA{255, 80, 80, 255}}
	// Missing instrument row label style
	MissingInstStyle = ButtonStyle{Fill: colError, Border: colButtonBorder}
	// Mute/Solo active styles for drum row buttons
	MuteActiveStyle = ButtonStyle{Fill: colMuteActive, Border: colMuteActiveBdr}
	SoloActiveStyle = ButtonStyle{Fill: colSoloActive, Border: colSoloActiveBdr}
	// EQ band mute button style (red when active, gray when inactive)
	EQMuteButtonStyle       = ButtonStyle{Fill: color.RGBA{80, 80, 80, 255}, Border: colButtonBorder}
	EQMuteButtonActiveStyle = ButtonStyle{Fill: color.RGBA{180, 50, 50, 255}, Border: colButtonBorder}
	// EQ filter (HPF/LPF) toggle button style (teal when active)
	EQFilterButtonActiveStyle = ButtonStyle{Fill: color.RGBA{40, 160, 160, 255}, Border: color.RGBA{80, 220, 220, 255}}

	TransportPlayStyle     = ButtonStyle{Fill: colDropdown, Border: colTransportBorder}
	TransportStopStyle     = ButtonStyle{Fill: colDropdown, Border: colTransportBorder}
	TransportIncStyle      = ButtonStyle{Fill: colDropdown, Border: colTransportBorder}
	TransportDecStyle      = ButtonStyle{Fill: colDropdown, Border: colTransportBorder}
	TransportMiscStyle     = ButtonStyle{Fill: colDropdown, Border: colTransportBorder}
	TransportFollowOnStyle = ButtonStyle{Fill: color.RGBA{40, 60, 80, 255}, Border: color.NRGBA{100, 180, 255, 80}}
	FABStyle               = ButtonStyle{Fill: color.RGBA{55, 180, 100, 255}, Border: color.NRGBA{255, 255, 255, 30}}
	MobileRowLabelStyle    = ButtonStyle{Fill: color.RGBA{28, 28, 36, 255}, Border: color.NRGBA{255, 255, 255, 8}}

	DrumCellUI = DrumCellStyle{
		On:        colStep,
		Off:       colStepOff,
		Highlight: colHighlight,
		Border:    colStepBorder,
	}

	// instColors maps instrument IDs to their display colors.
	instColors = map[string]color.Color{
		"snare": color.RGBA{235, 75, 90, 255},
		"kick":  color.RGBA{55, 210, 110, 255},
		"hihat": color.RGBA{240, 200, 50, 255},
		"tom":   color.RGBA{75, 110, 235, 255},
		"clap":  color.RGBA{215, 75, 200, 255},
	}

	// palette used for user-loaded instruments or any ids not in instColors.
	customPalette = []color.Color{
		color.RGBA{80, 200, 200, 255}, // cyan
		color.RGBA{200, 120, 80, 255}, // orange
		color.RGBA{120, 80, 200, 255}, // purple
		color.RGBA{200, 80, 120, 255}, // pink
		color.RGBA{80, 200, 120, 255}, // spring green
	}

	customColors    = map[string]color.Color{}
	nextCustomColor int
)
