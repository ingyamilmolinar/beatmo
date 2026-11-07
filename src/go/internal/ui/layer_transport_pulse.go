package ui

import "github.com/hajimehoshi/ebiten/v2"

// On top of the toolbar so the pulse animates without invalidating the cached toolbar. Playback has NO halo:
// playing is the play key's ACTIVE state, and a square halo around a rounded key broke "the focus ring is the only outline".
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
	// Mobile also draws a static armed ring inside the toolbar cache; this pulse adds "live and waiting".
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
