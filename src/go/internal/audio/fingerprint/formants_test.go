package fingerprint

import (
	"math"
	"testing"
)

func TestSpectralEnvelope_FindsResonance(t *testing.T) {
	sr := 48000
	w := synthFormantTone(sr, 2.0, 300, 2400, 8.0)
	env := computeSpectralEnvelope(sustainWindow(w), DefaultAnalysisConfig())
	if env.BridgeHillHz < 1800 || env.BridgeHillHz > 3000 {
		t.Fatalf("BridgeHillHz=%.0f, want ~2400", env.BridgeHillHz)
	}
	if len(env.Formants) == 0 {
		t.Fatal("no formants found")
	}
}

func TestSpectralEnvelope_SilentInput(t *testing.T) {
	sr := 48000
	w := synthFormantTone(sr, 2.0, 300, 2400, 8.0)
	// zero all samples → silent
	for i := range w.Samples {
		w.Samples[i] = 0
	}
	env := computeSpectralEnvelope(w, DefaultAnalysisConfig())
	if env.BridgeHillHz != 0 {
		t.Errorf("silent input: BridgeHillHz=%.1f, want 0", env.BridgeHillHz)
	}
	if len(env.Formants) != 0 {
		t.Errorf("silent input: got %d formants, want 0", len(env.Formants))
	}
}

func TestSpectralEnvelope_NoBridgeHillOutsideRange(t *testing.T) {
	sr := 48000
	// formant at 500 Hz — outside the 1.5-4 kHz bridge hill range
	w := synthFormantTone(sr, 2.0, 100, 500, 8.0)
	env := computeSpectralEnvelope(sustainWindow(w), DefaultAnalysisConfig())
	if env.BridgeHillHz != 0 {
		t.Errorf("formant at 500 Hz: BridgeHillHz=%.0f, want 0 (outside 1.5-4 kHz)", env.BridgeHillHz)
	}
}

func TestSpectralEnvelope_FormantBandwidth(t *testing.T) {
	sr := 48000
	w := synthFormantTone(sr, 2.0, 300, 2400, 8.0)
	env := computeSpectralEnvelope(sustainWindow(w), DefaultAnalysisConfig())
	if len(env.Formants) == 0 {
		t.Fatal("no formants")
	}
	// every formant must have a non-negative bandwidth, capped to a sane max
	for i, f := range env.Formants {
		if f.BandwidthHz < 0 {
			t.Errorf("Formant[%d].BandwidthHz=%.1f, must be >= 0", i, f.BandwidthHz)
		}
		if math.IsNaN(f.Hz) || math.IsInf(f.Hz, 0) {
			t.Errorf("Formant[%d].Hz is not finite: %v", i, f.Hz)
		}
	}
	// I1: the formant nearest 2400 Hz must have a bounded, sane bandwidth.
	// A broad/monotone peak could walk to bin 0 without the cap, yielding multi-kHz bogus BW.
	for _, f := range env.Formants {
		if f.Hz > 2200 && f.Hz < 2600 {
			if f.BandwidthHz <= 20 || f.BandwidthHz >= 2000 {
				t.Errorf("2400 Hz formant BandwidthHz=%.1f, want 20 < BW < 2000", f.BandwidthHz)
			}
			break
		}
	}
}

func TestSpectralEnvelope_FormantsSortedByGain(t *testing.T) {
	sr := 48000
	w := synthFormantTone(sr, 2.0, 300, 2400, 8.0)
	env := computeSpectralEnvelope(sustainWindow(w), DefaultAnalysisConfig())
	for i := 1; i < len(env.Formants); i++ {
		if env.Formants[i].GainDB > env.Formants[i-1].GainDB {
			t.Errorf("Formants not sorted by GainDB descending: [%d]=%.2f > [%d]=%.2f",
				i, env.Formants[i].GainDB, i-1, env.Formants[i-1].GainDB)
		}
	}
}

func TestSpectralEnvelope_EmptyWave(t *testing.T) {
	env := computeSpectralEnvelope(synthTone(48000, 0.0001, 440, []float64{1}), DefaultAnalysisConfig())
	// should not panic; BridgeHillHz should be 0
	if math.IsNaN(env.BridgeHillHz) {
		t.Error("BridgeHillHz is NaN on near-empty wave")
	}
}
