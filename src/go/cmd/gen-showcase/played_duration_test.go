//go:build !test && !js

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

// The engine used to apply a melodic voice's node duration as its playback
// RATE, so a note of duration 0.5 played an octave high and 0.25 two octaves
// high. Every template writes note lengths into duration, so every eighth and
// sixteenth of every melodic line was transposed — Ave Maria's vocal line leapt
// by octaves. TestTemplateNodesSoundAtTheirNotatedPitch could not see it: it
// rendered the recipe at the node's pitch and never applied the duration.
// These tests go through the PLAYBACK path (audio.RenderAsPlayed, the voice
// the mixer receives).

// gateFadeSamples bounds the release a shortened note ends with (the engine's
// gateFadeMs at the highest sample rate the engine runs).
const gateFadeSamples = 48000 * 10 / 1000

// TestNodeDurationNeverChangesPitch: a node's duration says how long a note
// lasts, never what note it is. For a pitch-aware voice the note played at
// duration d must be the SAME waveform as at duration 1, sample for sample, up
// to where it is cut — which no transposition, however small, survives. The
// property is checked exactly rather than through an f0 detector: on a short
// buffer the detector's 2.7 Hz bins and octave slips (57 cents at 80 Hz)
// swamp the thing being asserted.
//
// A resampled voice (drums, FM, non-melodic pads) is pitched and sized by its
// playback rate BY DESIGN, so a template must never give one a duration.
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

// TestAveMariaVoiceSoundsAtTheNotatedPitch measures what the listener
// reported: every note of Ave Maria's vocal line, rendered as the engine plays
// it (its real node pitch AND duration), against the frequency the score
// writes. Before the fix only the quarter notes were in tune; eighths read
// +1200 cents, sixteenths +2400, dotted eighths +498 and dotted quarters -702.
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
		// The played voice, converted (windowed sinc) to the rate the f0
		// detector is calibrated at: its frames are a fixed 4096 SAMPLES, so
		// at the engine's 48 kHz a half note's 0.2 s sustain window catches
		// more of the vibrato and read +32 cents where 44.1 kHz read +25, for a
		// note a fine autocorrelation puts at -5 cents at both rates.
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
