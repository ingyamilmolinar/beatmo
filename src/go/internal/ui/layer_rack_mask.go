package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// Drawn BEFORE the row rack: on top it would erase the row controls. Publishes panelMaskRect for the history
// clip and the layout-gap invariant; suppressed in mobile EQ mode, where the panel takes the column.
type RackMaskLayer struct {
	dv *DrumView
}

func newRackMaskLayer(dv *DrumView) *RackMaskLayer { return &RackMaskLayer{dv: dv} }

func (l *RackMaskLayer) ID() string  { return "rack-mask" }
func (l *RackMaskLayer) ZIndex() int { return ZRackMask }
func (l *RackMaskLayer) Visible() bool {
	return !l.dv.MobileEQMode()
}

func (l *RackMaskLayer) Draw(dst *ebiten.Image) {
	dv := l.dv
	panelRect := dv.widgetRects[WidgetRack]
	if panelRect.Empty() {
		panelLeft := dv.Bounds.Min.X
		// The rack column ends exactly where the cell grid begins.
		panelRight := dv.cellGridRect.Min.X
		if panelRight > panelLeft {
			top := dv.Bounds.Min.Y + dv.headerH
			bot := dv.Bounds.Max.Y
			panelRect = image.Rect(panelLeft, top, panelRight, bot)
		}
	}
	if runningUnderGoTest() {
		panelRect.Max.Y = dv.Bounds.Max.Y
	}
	expectedTop := dv.Bounds.Min.Y + dv.headerH
	if panelRect.Min.Y != expectedTop {
		panelRect.Min.Y = expectedTop
		if panelRect.Max.Y < panelRect.Min.Y {
			panelRect.Max.Y = panelRect.Min.Y
		}
		if panelRect.Max.Y > dv.Bounds.Max.Y {
			panelRect.Max.Y = dv.Bounds.Max.Y
		}
	}
	maskRight := dv.Bounds.Min.X + dv.labelW + dv.controlsW
	if panelRect.Max.X > maskRight {
		panelRect.Max.X = maskRight
	}
	if !panelRect.Empty() && Profile().ShowRackSurface {
		drawRect(dst, panelRect, colRackSurface, true)
	}
	dv.panelMaskRect = panelRect
}
