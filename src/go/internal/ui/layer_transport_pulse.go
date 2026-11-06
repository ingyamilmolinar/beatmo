package ui

import "github.com/hajimehoshi/ebiten/v2"

// TransportPulseLayer paints the record button's pulse halo while recording.
// Multi-pass rect inset gives a soft falloff. Drawn on top of the toolbar so
// the pulse can animate without invalidating the cached toolbar render.
//
// Playback has NO halo: a running transport is the play key's ACTIVE state —
// DESIGN.md "Interaction states" (accent tint + 2 px bottom accent bar, via
// Button.SetToggled in TransportZone.SetPlaying). The old play halo stroked
// square rects outside a rounded key, which read as a square frame around
// Pause and broke "the focus ring is the only outline".
//
// Z = ZTransportPulse (140) — above EQPanel (130), below ViewSwitch (150).
type TransportPulseLayer struct {
	dv *DrumView
}

func newTransportPulseLayer(dv *DrumView) *TransportPulseLayer {
	return &TransportPulseLayer{dv: dv}
}

func (l *TransportPulseLayer) ID() string  { return "transport-pulse" }
func (l *TransportPulseLayer) ZIndex() int { return ZTransportPulse }
func (l *TransportPulseLayer) Visible() bool {
	return l.dv.IsRecording() && l.dv.transportZone != nil
}

func (l *TransportPulseLayer) Draw(dst *ebiten.Image) {
	dv := l.dv
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
