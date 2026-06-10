package audio

import "testing"

// TestModularFilterResponse_LowPass — a low-pass synth filter must pass low
// frequencies (~0 dB) and attenuate well above the cutoff. Mirrors the C
// mod_biquad_set coefficients so the Synth-tab filter preview matches the
// audio the modular voice actually renders.
func TestModularFilterResponse_LowPass(t *testing.T) {
	pts := ModularFilterResponse(0 /*LP*/, 1000, 0.707, 48000, 128, 20, 20000)
	if len(pts) == 0 {
		t.Fatalf("no response points")
	}
	low := gainNearHz(t, pts, 100)
	high := gainNearHz(t, pts, 10000)
	if low < -3 {
		t.Errorf("LP passband too attenuated at 100Hz: %.2f dB (want ~0)", low)
	}
	if high > low-12 {
		t.Errorf("LP did not roll off above cutoff: 100Hz=%.2f dB 10kHz=%.2f dB (want 10kHz much lower)", low, high)
	}
}

// TestModularFilterResponse_HighPass — high-pass is the mirror image.
func TestModularFilterResponse_HighPass(t *testing.T) {
	pts := ModularFilterResponse(1 /*HP*/, 1000, 0.707, 48000, 128, 20, 20000)
	low := gainNearHz(t, pts, 100)
	high := gainNearHz(t, pts, 10000)
	if high < -3 {
		t.Errorf("HP passband too attenuated at 10kHz: %.2f dB (want ~0)", high)
	}
	if low > high-12 {
		t.Errorf("HP did not roll off below cutoff: 100Hz=%.2f dB 10kHz=%.2f dB (want 100Hz much lower)", low, high)
	}
}

// TestModularFilterResponse_CutoffTracks — raising the cutoff must shift the
// low-pass roll-off upward, so the gain at a fixed probe frequency above the
// old cutoff increases. This is what the preview's "filter plot tracks cutoff"
// assertion relies on.
func TestModularFilterResponse_CutoffTracks(t *testing.T) {
	probe := 4000.0
	lowCut := gainNearHz(t, ModularFilterResponse(0, 800, 0.707, 48000, 256, 20, 20000), probe)
	highCut := gainNearHz(t, ModularFilterResponse(0, 8000, 0.707, 48000, 256, 20, 20000), probe)
	if !(highCut > lowCut) {
		t.Errorf("raising cutoff did not raise gain at %.0fHz: cut=800→%.2f dB cut=8000→%.2f dB", probe, lowCut, highCut)
	}
}

// gainNearHz returns the GainDB of the response point closest to targetHz.
func gainNearHz(t *testing.T, pts []FreqResponsePoint, targetHz float64) float64 {
	t.Helper()
	best := pts[0]
	bestD := 1e18
	for _, p := range pts {
		d := p.FreqHz - targetHz
		if d < 0 {
			d = -d
		}
		if d < bestD {
			bestD, best = d, p
		}
	}
	return best.GainDB
}
