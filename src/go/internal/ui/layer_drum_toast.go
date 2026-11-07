package ui

import "github.com/hajimehoshi/ebiten/v2"

// The grid pane's own toast is off-screen while a non-Pads view is up, exactly when the user waits on a
// disabled Play during warm-up. One toast object; this layer only supplies the other host rect.
type DrumToastLayer struct {
	dv *DrumView
}

func newDrumToastLayer(dv *DrumView) *DrumToastLayer { return &DrumToastLayer{dv: dv} }

func (l *DrumToastLayer) ID() string  { return "drumview-toast" }
func (l *DrumToastLayer) ZIndex() int { return ZDrumToast }

// The negation of gridPaneVisibleForToast, not a new per-view-mode switch.
func (l *DrumToastLayer) Visible() bool {
	return Profile().IsMobile() && !l.dv.gridPaneVisibleForToast() && l.dv.logsToast.active()
}

// Below the transport header, which top-centring against the pane would cover. No HitArea, so it never swallows a tap.
func (l *DrumToastLayer) Draw(dst *ebiten.Image) {
	l.dv.logsToast.draw(dst, l.dv.BelowHeaderRect())
}
