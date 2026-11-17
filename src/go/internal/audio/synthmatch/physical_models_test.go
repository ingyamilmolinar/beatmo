//go:build !test && !js

package synthmatch

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// These tests run on the NATIVE build (real CGo DSP) — under -tags test the
// renderer is a no-op stub, so the //go:build !test guard is required. They pin
// the physical-model oscillators in src/c/modular.c (bowed string osc_type:7,
// brass osc_type:8, reed osc_type:9, air-jet flute osc_type:10) and the
// Karplus-Strong guitar so a future C edit that breaks self-oscillation, blows
// up the feedback loop, or detunes the pitch fails loudly.

const pmSampleRate = 48000

func waveStats(w wave.Wave) (rms, peak float64, finite bool) {
	finite = true
	var sumSq float64
	for _, s := range w.Samples {
		if math.IsNaN(s) || math.IsInf(s, 0) {
			finite = false
			continue
		}
		if a := math.Abs(s); a > peak {
			peak = a
		}
		sumSq += s * s
	}
	if len(w.Samples) > 0 {
		rms = math.Sqrt(sumSq / float64(len(w.Samples)))
	}
	return
}

// lateStable guards against runaway feedback: the peak of the last 20% of the
// render must not exceed 4× the peak of the first 80% (a self-oscillating
// waveguide settles into a limit cycle; an unstable one grows without bound).
func lateStable(w wave.Wave) bool {
	n := len(w.Samples)
	if n < 100 {
		return true
	}
	cut := n * 8 / 10
	var earlyPeak, latePeak float64
	for i, s := range w.Samples {
		if math.IsNaN(s) || math.IsInf(s, 0) {
			return false
		}
		a := math.Abs(s)
		if i < cut {
			if a > earlyPeak {
				earlyPeak = a
			}
		} else if a > latePeak {
			latePeak = a
		}
	}
	if earlyPeak < 1e-6 {
		return latePeak < 1e-3 // silent early ⇒ must not erupt late
	}
	return latePeak <= 4.0*earlyPeak
}

// TestPhysicalModelInstrumentsRenderCleanly pins the SHIPPED physical-model
// instruments: each must render finite, bounded, stable, audible, and on pitch.
func TestPhysicalModelInstrumentsRenderCleanly(t *testing.T) {
	cases := []struct {
		id     instrument.ID
		pitch  float64 // node pitch, semitones
		tolPct float64
	}{
		{roster.Violin, 17, 7},      // bowed-string waveguide (osc_type:7)
		{roster.Cello, -7, 9},       // bowed-string waveguide, low note
		{roster.Oboe, 12, 9},        // reed waveguide (osc_type:9)
		{roster.GuitarNylon, 15, 7}, // Karplus-Strong + warm nylon body (body_model:2)
		{roster.GuitarSteel, 15, 7}, // Karplus-Strong + bright steel body (body_model:4)
		{roster.Harp, 15, 7},        // Karplus-Strong celtic harp (bright, long ring)
	}
	for _, c := range cases {
		t.Run(string(c.id), func(t *testing.T) {
			w, err := RenderInstrument(c.id, c.pitch, pmSampleRate, 2.0)
			if err != nil {
				t.Fatalf("render %s: %v", c.id, err)
			}
			rms, peak, finite := waveStats(w)
			if !finite {
				t.Fatalf("%s: produced NaN/Inf", c.id)
			}
			if peak > 1.5 {
				t.Errorf("%s: peak %.3f too hot (clipping/instability)", c.id, peak)
			}
			if rms < 0.005 {
				t.Fatalf("%s: silent (RMS %.5f) — self-oscillation failed", c.id, rms)
			}
			if !lateStable(w) {
				t.Errorf("%s: unstable — late amplitude blows up", c.id)
			}
			// The note a node at this pitch promises: the cello's osc_octave -1
			// puts it an octave under the node's A3-relative pitch.
			offset, _ := audio.InstrumentPitchInfo(c.id)
			wantF0 := 220 * math.Pow(2, (c.pitch+offset)/12)
			f0 := fingerprint.DetectF0(w)
			tol := wantF0 * c.tolPct / 100
			// Allow octave-up detection (a strong 2nd harmonic is common and benign).
			if math.Abs(f0-wantF0) > tol && math.Abs(f0-2*wantF0) > 2*tol {
				t.Errorf("%s: F0 %.0f Hz, want %.0f±%.0f Hz (or its octave)", c.id, f0, wantF0, tol)
			}
		})
	}
}

// TestWaveguideOscTypesAreFinite pins the RAW C waveguides (osc_type 7..10):
// EVERY one MUST produce finite, bounded, non-runaway output (the guards in
// modular.c are the completeness guarantee — TestCompleteness_RecipeRenderPresets
// HaveFiniteOutput renders osc_type at its max, so an unguarded blow-up SIGSEGVs).
// The two shipped self-oscillators (bowed=7, reed=9) must additionally sound.
func TestWaveguideOscTypesAreFinite(t *testing.T) {
	cases := []struct {
		oscType   int
		name      instrument.ID
		mustSound bool
	}{
		{7, "bowed", true},
		{8, "brass", false}, // implemented but not yet a clean self-oscillator
		{9, "reed", true},
		{10, roster.Flute, false}, // air-jet oscillates only in a narrow regime; not wired
		{11, roster.Sax, true},    // Saxofony reed-cone — all-harmonic, self-oscillating
	}
	for _, c := range cases {
		t.Run(string(c.name), func(t *testing.T) {
			over := map[string]float64{"osc_enabled": 1, "osc_type": float64(c.oscType)}
			w, err := RenderWithParams(roster.Modular, 4, pmSampleRate, 1.5, over)
			if err != nil {
				t.Fatalf("render osc_type %d: %v", c.oscType, err)
			}
			rms, peak, finite := waveStats(w)
			if !finite {
				t.Fatalf("osc_type %d (%s): NaN/Inf", c.oscType, c.name)
			}
			if peak > 2.0 {
				t.Errorf("osc_type %d (%s): peak %.3f unbounded", c.oscType, c.name, peak)
			}
			if !lateStable(w) {
				t.Errorf("osc_type %d (%s): unstable/runaway feedback", c.oscType, c.name)
			}
			if c.mustSound && rms < 0.005 {
				t.Errorf("osc_type %d (%s): silent (RMS %.5f) — should self-oscillate", c.oscType, c.name, rms)
			}
		})
	}
}
