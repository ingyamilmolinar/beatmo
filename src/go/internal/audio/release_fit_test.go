//go:build !test && !js

package audio

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// The release starts at most half-way through the voice; its ramp fits the half it is given, so a long release fades
// out inside the note instead of being hard-cut at the end, and one dial means the same on every instrument.
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
