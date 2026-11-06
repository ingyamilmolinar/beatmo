package ui

import "github.com/hajimehoshi/ebiten/v2"

// DrumToastLayer paints the mobile fallback status toast into the drum pane
// when a non-Pads audio tab is active. The grid pane's own "grid-toast"
// layer (GZToast, grid_tree.go / drawGridToast) is invisible on that
// surface — exactly when the user is staring at a disabled Play button
// waiting for warm-up (a bottom nav switches view modes, and the grid pane
// is not what's on screen in EQ/Wave/Spectrum/Levels/Chain/Synth/Sampler).
//
// One toast object (dv.logsToast), one draw method (logsToast.draw); this
// layer supplies only the alternate host rect and visibility gate — see
// toastHostRect (grid_pane_draw.go) for the placement decision this layer
// mirrors.
//
// Z = ZDrumToast — topmost in dv.audioTree, so it draws over the EQ panel,
// the mobile view-switch segmented control, and the row↔EQ divider.
type DrumToastLayer struct {
	dv *DrumView
}

func newDrumToastLayer(dv *DrumView) *DrumToastLayer { return &DrumToastLayer{dv: dv} }

func (l *DrumToastLayer) ID() string  { return "drumview-toast" }
func (l *DrumToastLayer) ZIndex() int { return ZDrumToast }

// Visible gates on: mobile, the grid pane is NOT the active surface (so the
// grid-pane toast would be off-screen), and a task/error is actually
// showing. Mirrors gridPaneVisibleForToast's negation rather than a new
// per-view-mode switch.
func (l *DrumToastLayer) Visible() bool {
	return Profile().IsMobile() && !l.dv.gridPaneVisibleForToast() && l.dv.logsToast.active()
}

// Draw paints the toast at the top-centre of the drum pane BELOW the
// transport header (BelowHeaderRect) — the raw Bounds' top edge is the
// Play/Stop/Record/BPM row, and top-centring against the full pane would
// cover it. The toast is passive — it registers no HitArea (this is a plain
// Layer, not a Zone) so it can never swallow a tap.
func (l *DrumToastLayer) Draw(dst *ebiten.Image) {
	l.dv.logsToast.draw(dst, l.dv.BelowHeaderRect())
}
