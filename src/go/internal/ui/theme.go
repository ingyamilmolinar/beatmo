package ui

import "image/color"

var (
	colBGTop    = color.RGBA{30, 30, 30, 255}
	colBGBottom = color.RGBA{20, 20, 20, 255}
	colGridLine = color.RGBA{50, 50, 50, 255}

	colButtonBorder = color.RGBA{220, 220, 220, 255}
	colPlayButton   = color.RGBA{60, 170, 90, 255}
	colStopButton   = color.RGBA{170, 60, 60, 255}
	colBPMBox       = color.RGBA{45, 45, 45, 255}
	colLenDec       = color.RGBA{70, 130, 180, 255}
	colLenInc       = color.RGBA{70, 70, 180, 255}
	colDropdown     = color.RGBA{80, 80, 80, 255}
	colDropdownEdge = color.RGBA{240, 240, 120, 255}
	colError        = color.RGBA{200, 60, 60, 255}

	colStep       = color.RGBA{0, 160, 200, 255}
	colStepOff    = color.RGBA{25, 25, 25, 255}
	colStepBorder = color.RGBA{60, 60, 60, 255}
	colHighlight  = color.RGBA{240, 240, 40, 255}

	colTimelineTotal  = color.RGBA{40, 40, 40, 255}
	colTimelineView   = color.RGBA{0, 160, 200, 255}
	colTimelineCursor = color.RGBA{240, 240, 40, 255}

	NodeUI   = NodeStyle{Radius: 16, Fill: color.RGBA{80, 80, 80, 255}, Border: color.RGBA{220, 220, 220, 255}}
	SignalUI = SignalStyle{Radius: 6, Color: color.RGBA{0, 160, 200, 255}}
	EdgeUI   = EdgeStyle{Color: color.RGBA{220, 220, 220, 120}, Thickness: 2, ArrowSize: 8}

	PlayButtonStyle = ButtonStyle{Fill: colPlayButton, Border: colButtonBorder}
	StopButtonStyle = ButtonStyle{Fill: colStopButton, Border: colButtonBorder}
	BPMBoxStyle     = TextInputStyle{Fill: colBPMBox, Border: colButtonBorder, Cursor: color.White}
	BPMDecStyle     = ButtonStyle{Fill: colLenDec, Border: colButtonBorder}
	BPMIncStyle     = ButtonStyle{Fill: colLenInc, Border: colButtonBorder}
	LenDecStyle     = ButtonStyle{Fill: colLenDec, Border: colButtonBorder}
	LenIncStyle     = ButtonStyle{Fill: colLenInc, Border: colButtonBorder}
	InstButtonStyle = ButtonStyle{Fill: colBPMBox, Border: colButtonBorder}
	UploadBtnStyle  = ButtonStyle{Fill: colBPMBox, Border: colButtonBorder}
	DropdownStyle   = ButtonStyle{Fill: colDropdown, Border: colDropdownEdge}

	DrumCellUI = DrumCellStyle{
		On:        colStep,
		Off:       colStepOff,
		Highlight: colHighlight,
		Border:    colStepBorder,
	}

	// instColors maps instrument IDs to their display colors.
	instColors = map[string]color.Color{
		"snare": color.RGBA{200, 80, 80, 255},
		"kick":  color.RGBA{80, 200, 80, 255},
		"hihat": color.RGBA{200, 200, 80, 255},
		"tom":   color.RGBA{80, 80, 200, 255},
		"clap":  color.RGBA{200, 80, 200, 255},
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
