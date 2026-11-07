//go:build !test && !js

package audio

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Every analysis tool renders through RenderAsShipped, so a drift from real playback means every comparison measures a
// sound nobody hears.
func TestRenderAsShippedMatchesPlayback(t *testing.T) {
	for _, id := range roster.All {
		d, ok := EnsureInstrumentDoc(id)
		if !ok {
			t.Fatalf("%s: no document", id)
		}
		for _, pitch := range []float64{0, 7} {
			want, sr := RenderAsPlayed(id, pitch, 1, 60)
			got := RenderAsShipped(d, sr, pitch)
			if len(want) == 0 || peakAbs64(want) == 0 {
				t.Errorf("%s pitch %+v: playback produced no sound — the comparison would be vacuous", id, pitch)
				continue
			}
			if len(got) != len(want) {
				t.Errorf("%s pitch %+v: %d samples, playback %d", id, pitch, len(got), len(want))
				continue
			}
			for i := range got {
				if math.Float64bits(got[i]) != math.Float64bits(want[i]) {
					t.Errorf("%s pitch %+v: sample %d = %v, playback %v", id, pitch, i, got[i], want[i])
					break
				}
			}
		}
	}
}

func peakAbs64(x []float64) float64 {
	var p float64
	for _, v := range x {
		p = math.Max(p, math.Abs(v))
	}
	return p
}
