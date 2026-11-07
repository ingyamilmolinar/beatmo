//go:build !test

package main

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// Through the PLAYBACK path (audio.RenderAsPlayed): rendering at the node's pitch without its duration cannot see a
// duration that transposes.

// gateFadeSamples bounds the release a shortened note ends with (the engine's
// gateFadeMs at the highest sample rate the engine runs).
const gateFadeSamples = 48000 * 10 / 1000

// A note at duration d must be the SAME waveform as at duration 1 up to where it is cut, which no transposition
// survives; an f0 detector's bins and octave slips on a short buffer would swamp it. A resampled voice is sized by rate
// by design, so a template never gives one a duration.
func TestNodeDurationNeverChangesPitch(t *testing.T) {
	for _, spec := range templateSpecs() {
		spec := spec
		t.Run(spec.Stem, func(t *testing.T) {
			d := readTemplate(t, spec.Stem)
			rowOf := rowIndexer(d)
			type key struct {
				inst       string
				pitch, dur float64
			}
			seen := map[key]bool{}
			for i, n := range d.Nodes {
				if n.Type != "regular" {
					continue
				}
				inst := d.Instruments[rowOf(i)].ID
				if !audio.IsMelodicInstrument(inst) {
					if n.Duration != 0 && n.Duration != 1 {
						t.Errorf("node %d on %s (resampled voice) carries duration %v — it would play %+.0f cents off",
							i, inst, n.Duration, -1200*math.Log2(n.Duration))
					}
					continue
				}
				k := key{string(inst), n.Pitch, n.Duration}
				if n.Duration == 0 || n.Duration == 1 || seen[k] {
					continue
				}
				seen[k] = true
				ref, _ := audio.RenderAsPlayed(inst, n.Pitch, 1, 4)
				got, _ := audio.RenderAsPlayed(inst, n.Pitch, n.Duration, 4)
				if len(ref) == 0 || len(got) == 0 {
					t.Errorf("%s pitch %+.0f: nothing rendered", inst, n.Pitch)
					continue
				}
				if len(got) > len(ref) {
					t.Errorf("%s pitch %+.0f duration %.3f: %d samples, longer than the full note (%d)", inst, n.Pitch, n.Duration, len(got), len(ref))
					continue
				}
				if n.Duration < 1 {
					if want := int(float64(len(ref)) * n.Duration); absInt(len(got)-want) > 1 {
						t.Errorf("%s pitch %+.0f duration %.3f: lasts %d samples, want %d", inst, n.Pitch, n.Duration, len(got), want)
					}
				}
				for j := 0; j < len(got)-gateFadeSamples; j++ {
					if got[j] != ref[j] {
						t.Errorf("%s pitch %+.0f duration %.3f: sample %d is %v, %v at duration 1 — the shortened note is not the same note",
							inst, n.Pitch, n.Duration, j, got[j], ref[j])
						break
					}
				}
			}
		})
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// playedAtRate converts a played voice to rate with the engine's own sample
// converter (audio.ResampleToRate, windowed sinc).
func playedAtRate(s []float64, from, rate int) []float64 {
	f := make([]float32, len(s))
	for i, v := range s {
		f[i] = float32(v)
	}
	conv := audio.ResampleToRate(f, from, rate)
	out := make([]float64, len(conv))
	for i, v := range conv {
		out[i] = float64(v)
	}
	return out
}

// avePitchTolCents is the measurement tolerance for a sung note. No octave
// folding: an octave is exactly the error this test exists to catch.
const avePitchTolCents = 30.0

// Every note of the vocal line, rendered as the engine plays it (pitch AND duration), against the score's frequency.
func TestAveMariaVoiceSoundsAtTheNotatedPitch(t *testing.T) {
	const inst = roster.VoiceSoprano
	d := readTemplate(t, "schubert-ave-maria")
	rowOf := rowIndexer(d)
	offset, _ := audio.InstrumentPitchInfo(inst)
	type key struct{ pitch, dur float64 }
	seen := map[key]bool{}
	measured := 0
	for i, n := range d.Nodes {
		if n.Type != "regular" || d.Instruments[rowOf(i)].ID != inst || seen[key{n.Pitch, n.Duration}] {
			continue
		}
		seen[key{n.Pitch, n.Duration}] = true
		// Converted to the rate the f0 detector is calibrated at: its frames are a fixed 4096 SAMPLES, so at 48 kHz a half
		// note's sustain window catches more vibrato and reads sharp.
		s, sr := audio.RenderAsPlayed(inst, n.Pitch, n.Duration, 1.5)
		w := wave.Wave{Samples: playedAtRate(s, sr, wavref.Rate), SampleRate: wavref.Rate}
		est := fingerprint.DetectF0Detailed(w)
		if usable, _ := fingerprint.UsableAsPitch(w, est); !usable || est.Confidence < 0.5 {
			continue
		}
		measured++
		want := 220 * math.Pow(2, (n.Pitch+offset)/12)
		if c := 1200 * math.Log2(est.Hz/want); math.Abs(c) > avePitchTolCents {
			t.Errorf("voice note at node pitch %+.0f, duration %.3f: plays %.1f Hz, the score writes %.1f Hz (%+.0f cents)",
				n.Pitch, n.Duration, est.Hz, want, c)
		}
	}
	if measured < 10 {
		t.Fatalf("only %d voice notes could be measured — the test proved little", measured)
	}
}
