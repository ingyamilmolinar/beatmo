package ui

import "github.com/hajimehoshi/ebiten/v2"

// TransportPulseLayer paints the play and record button pulse halos.
// Multi-pass rect inset gives a soft falloff (DESIGN.md "Cushioned
// elevation"). Drawn on top of the toolbar so the pulse can animate without
// invalidating the cached toolbar render.
//
// Z = ZTransportPulse (140) — above EQPanel (130), below ViewSwitch (150).
type TransportPulseLayer struct {
	dv *DrumView
}

func newTransportPulseLayer(dv *DrumView) *TransportPulseLayer {
	return &TransportPulseLayer{dv: dv}
}

func (l *TransportPulseLayer) ID() string    { return "transport-pulse" }
func (l *TransportPulseLayer) ZIndex() int   { return ZTransportPulse }
func (l *TransportPulseLayer) Visible() bool {
	dv := l.dv
	playing := dv.isPlaying && dv.playBtn() != nil && dv.playBtn().Icon == string(IconPause)
	recording := dv.IsRecording() && dv.transportZone != nil
	return playing || recording
}

func (l *TransportPulseLayer) Draw(dst *ebiten.Image) {
	dv := l.dv
	// Play halo — gated on the visible Pause icon to guard against state
	// drift where isPlaying=true but the icon was left as Play.
	if dv.isPlaying && dv.playBtn() != nil && dv.playBtn().Icon == string(IconPause) {
		if pr := dv.playBtn().Rect(); !pr.Empty() {
			peak := SinPulseAlpha(dv.frame, genAnimPlayheadPulse)
			for i := 1; i <= 3; i++ {
				a := uint8(int(peak) * (4 - i) / 4)
				if a == 0 {
					continue
				}
				drawRect(dst, pr.Inset(-i), WithAlpha(genColorDrumGlow, a), false)
			}
		}
	}
	// Record halo — destructive red, same falloff. Mobile additionally
	// renders a static destructive ring inside the toolbar cache
	// (drawRecordArmedRingOffset) for the discrete "armed" affordance —
	// this pulse adds the "live & waiting" energy.
	if dv.IsRecording() && dv.transportZone != nil {
		if rr := dv.transportZone.recordBtn.Rect(); !rr.Empty() {
			peak := SinPulseAlpha(dv.frame, genAnimPlayheadPulse)
			for i := 1; i <= 3; i++ {
				a := uint8(int(peak) * (4 - i) / 4)
				if a == 0 {
					continue
				}
				drawRect(dst, rr.Inset(-i), WithAlpha(genColorRecordActive, a), false)
			}
		}
	}
}
