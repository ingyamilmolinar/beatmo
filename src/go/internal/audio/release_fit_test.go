//go:build !test && !js

package audio

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// TestALongReleaseFadesOutInsideTheNote: the amp release starts at most
// half-way through the voice (the ADSR stage caps it so attack/decay/sustain
// keep the first half). It used to keep its FULL release time for the ramp,
// so a release longer than half the voice was still loud when the voice ended
// and hard-cut there (modular-pad at 120 BPM: 1.4 s release in a 0.5 s note
// ended at about −5 dB). That defect is why the Tail dial had a per-voice
// range cap — a knob meaning something different on every instrument. The
// ramp now fits the half it is given: the note fades out inside itself.
func TestALongReleaseFadesOutInsideTheNote(t *testing.T) {
	d, ok := EnsureInstrumentDoc(roster.ModularPad)
	if !ok {
		t.Fatal("modular-pad has no document")
	}
	sr := 48000
	buf := renderDocVoice(d, sr, 0, false)
	if len(buf) == 0 {
		t.Fatal("empty render")
	}
	rms := func(lo, hi int) float64 {
		s := 0.0
		for _, v := range buf[lo:hi] {
			s += float64(v) * float64(v)
		}
		return math.Sqrt(s / float64(hi-lo))
	}
	win := sr / 100 // 10 ms
	loud := 0.0
	for i := 0; i+win <= len(buf); i += win {
		loud = math.Max(loud, rms(i, i+win))
	}
	end := rms(len(buf)-win, len(buf))
	db := 20 * math.Log10(end/loud+1e-12)
	if db > -24 {
		t.Fatalf("the last 10 ms of modular-pad sit at %.1f dB under its loudest: the release is cut off, not faded (want ≤ −24 dB)", db)
	}
}
