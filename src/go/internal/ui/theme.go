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

	colStep       = color.RGBA{0, 160, 200, 255}
	colStepOff    = color.RGBA{25, 25, 25, 255}
	colStepBorder = color.RGBA{60, 60, 60, 255}
	colHighlight  = color.RGBA{240, 240, 40, 255}

	NodeUI   = NodeStyle{Radius: 16, Fill: color.RGBA{80, 80, 80, 255}, Border: color.RGBA{220, 220, 220, 255}}
	SignalUI = SignalStyle{Radius: 6, Color: color.RGBA{0, 160, 200, 255}}
	EdgeUI   = EdgeStyle{Color: color.RGBA{220, 220, 220, 255}, Thickness: 2, ArrowSize: 12}

	PlayButtonStyle = ButtonStyle{Fill: colPlayButton, Border: colButtonBorder}
	StopButtonStyle = ButtonStyle{Fill: colStopButton, Border: colButtonBorder}
	BPMBoxStyle     = TextInputStyle{Fill: colBPMBox, Border: colButtonBorder}
	BPMDecStyle     = ButtonStyle{Fill: colLenDec, Border: colButtonBorder}
	BPMIncStyle     = ButtonStyle{Fill: colLenInc, Border: colButtonBorder}
	LenDecStyle     = ButtonStyle{Fill: colLenDec, Border: colButtonBorder}
	LenIncStyle     = ButtonStyle{Fill: colLenInc, Border: colButtonBorder}
	InstButtonStyle = ButtonStyle{Fill: colBPMBox, Border: colButtonBorder}
	UploadBtnStyle  = ButtonStyle{Fill: colBPMBox, Border: colButtonBorder}

	DrumCellUI = DrumCellStyle{
		On:        colStep,
		Off:       colStepOff,
		Highlight: colHighlight,
		Border:    colStepBorder,
	}
)
