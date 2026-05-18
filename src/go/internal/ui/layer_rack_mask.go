package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// RackMaskLayer paints the rack-column surface elevation that visually
// groups the row controls. It draws BEFORE the row rack zone so the surface
// fill sits beneath row labels/buttons; painting it on top would erase the
// per-row controls (regression caught by
// TestDesktopRowRack_NoOpaqueOverpaintOnRackColumn).
//
// Z = ZRackMask (55) — between Background (50) and the primary zones.
//
// Also publishes panelMaskRect so other systems (history clip, layout-gap
// invariant) can read the rack column bounds without recomputing them.
//
// Suppressed when the mobile EQ mode is active (the EQ panel takes over the
// rack column on mobile and the surface would conflict with it).
type RackMaskLayer struct {
	dv *DrumView
}

func newRackMaskLayer(dv *DrumView) *RackMaskLayer { return &RackMaskLayer{dv: dv} }

func (l *RackMaskLayer) ID() string    { return "rack-mask" }
func (l *RackMaskLayer) ZIndex() int   { return ZRackMask }
func (l *RackMaskLayer) Visible() bool {
	return !l.dv.MobileEQMode()
}

func (l *RackMaskLayer) Draw(dst *ebiten.Image) {
	dv := l.dv
	panelRect := dv.widgetRects[WidgetRack]
	if panelRect.Empty() {
		panelLeft := dv.Bounds.Min.X
		panelRight := dv.timelineRect.Min.X
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
