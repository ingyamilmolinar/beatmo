package ui

import "github.com/hajimehoshi/ebiten/v2"

// After the transport controls, so it sits on the bottom bar's surface. Mobile only.
type ViewSwitchLayer struct {
	dv *DrumView
}

func newViewSwitchLayer(dv *DrumView) *ViewSwitchLayer { return &ViewSwitchLayer{dv: dv} }

func (l *ViewSwitchLayer) ID() string  { return "view-switch" }
func (l *ViewSwitchLayer) ZIndex() int { return ZViewSwitch }
func (l *ViewSwitchLayer) Visible() bool {
	return Profile().IsMobile() && l.dv.viewSwitchSegmented != nil &&
		!l.dv.viewSwitchSegmented.Rect().Empty()
}

func (l *ViewSwitchLayer) Draw(dst *ebiten.Image) {
	// Refreshed every frame: the instrument or channel can change between layouts.
	if idx := bottomNavSynthIndex(); idx >= 0 {
		l.dv.viewSwitchSegmented.SetSegmentDisabled(idx, l.dv.synthViewDisabled())
	}
	if idx := segmentIndexForViewMode(viewModeSampler); idx >= 0 {
		l.dv.viewSwitchSegmented.SetSegmentDisabled(idx, l.dv.samplerViewDisabled())
	}
	l.dv.viewSwitchSegmented.Draw(dst)
}
