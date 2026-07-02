package ui

import "github.com/hajimehoshi/ebiten/v2"

// ViewSwitchLayer paints the mobile segmented Pads/EQ/Wave/Spec/Mtr/Scope
// switch. Drawn after the transport controls so it renders on top of the
// bottom-action-bar surface. Mobile only — desktop layout zeros the rect.
//
// Z = ZViewSwitch (150).
type ViewSwitchLayer struct {
	dv *DrumView
}

func newViewSwitchLayer(dv *DrumView) *ViewSwitchLayer { return &ViewSwitchLayer{dv: dv} }

func (l *ViewSwitchLayer) ID() string    { return "view-switch" }
func (l *ViewSwitchLayer) ZIndex() int   { return ZViewSwitch }
func (l *ViewSwitchLayer) Visible() bool {
	return Profile().IsMobile() && l.dv.viewSwitchSegmented != nil &&
		!l.dv.viewSwitchSegmented.Rect().Empty()
}

func (l *ViewSwitchLayer) Draw(dst *ebiten.Image) {
	// Keep the Synth segment's greyed state fresh every frame even when no
	// relayout ran (the active instrument can change between layouts).
	if idx := bottomNavSynthIndex(); idx >= 0 {
		l.dv.viewSwitchSegmented.SetSegmentDisabled(idx, !l.dv.activeInstrumentHasSynth())
	}
	l.dv.viewSwitchSegmented.Draw(dst)
}
