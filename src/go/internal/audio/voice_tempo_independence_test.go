//go:build !test && !js

package audio

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Every PLAYED voice, at any pitch and Duration, is sample-identical at 60, 120 and 300 BPM. Each tempo renders fresh,
// so a cached buffer is never compared with itself.
func TestVoiceIsTheSameAtEveryTempo(t *testing.T) {
	t.Cleanup(func() { SetBPM(120) })
	cases := []struct{ pitch, dur float64 }{{0, 1}, {7, 1}, {-5, 0.5}}
	for _, id := range roster.All {
		d, ok := EnsureInstrumentDoc(id)
		if !ok {
			t.Fatalf("%s: no document", id)
		}
		for _, c := range cases {
			var ref []float64
			for _, bpm := range []int{60, 120, 300} {
				SetBPM(bpm)
				globalVoiceCache.ClearInstrument(id)
				got, sr := RenderAsPlayed(id, c.pitch, c.dur, 60)
				if len(got) == 0 || peakAbs64(got) == 0 {
					t.Fatalf("%s %+v at %d BPM: no sound — the comparison would be vacuous", id, c, bpm)
				}
				if c.pitch == 0 && c.dur == 1 && len(got) != VoiceFrames(d.Render.Seconds, sr) {
					t.Errorf("%s at %d BPM: %d samples, render.seconds %v is %d", id, bpm, len(got), d.Render.Seconds, VoiceFrames(d.Render.Seconds, sr))
				}
				if ref == nil {
					ref = got
					continue
				}
				if len(got) != len(ref) {
					t.Errorf("%s %+v: %d samples at %d BPM, %d at 60 BPM", id, c, len(got), bpm, len(ref))
					continue
				}
				for i := range got {
					if math.Float64bits(got[i]) != math.Float64bits(ref[i]) {
						t.Errorf("%s %+v: sample %d at %d BPM = %v, at 60 BPM %v", id, c, i, bpm, got[i], ref[i])
						break
					}
				}
			}
		}
	}
}

// No tempo in the cache key, so a BPM change keeps every rendered voice.
func TestTempoChangeReRendersNothing(t *testing.T) {
	t.Cleanup(func() { SetBPM(120) })
	sr := SampleRate()
	SetBPM(120)
	for _, id := range roster.All {
		if _, ok := EnsureInstrumentDoc(id); !ok {
			t.Fatalf("%s: no document", id)
		}
		RenderAsPlayed(id, 0, 1, 0.01)
	}
	SetBPM(173)
	for _, id := range roster.All {
		key, ok := liveVoiceKey(id, sr, 0, IsMelodicInstrument(id), false)
		if !ok {
			t.Fatalf("%s: no voice key", id)
		}
		if !globalVoiceCache.Has(key) {
			t.Errorf("%s: its voice left the cache when the tempo changed", id)
		}
	}
}
