//go:build !test && !js

package audio

import (
	"fmt"
	"math"
	"testing"
)

// TestKickCreakDiagnose isolates the "creaky door" artifact in kick-acoustic by
// rendering the seed with the reverb OFF vs ON and with the modal glide/modes
// varied, then measuring the TAIL (50-350 ms) for a creak signature: sustained
// mid-band (250-1500 Hz) energy that is amplitude-MODULATED over time (a creak
// is a warbly resonance, not a clean decay). Diagnostic only (always passes).
func TestKickCreakDiagnose(t *testing.T) {
	const sr = 44100
	const samples = sr * 6 / 10 // 0.6 s

	variants := []struct {
		name string
		over RecipeParams
	}{
		{"full (current)", nil},
		{"sat OFF", RecipeParams{"gen1_kick_sat": 0}},
		{"harmonic modes (stretch 0)", RecipeParams{"gen1_kick_mode_detune": 0}},
		{"high mode off (h4=0)", RecipeParams{"gen1_kick_h4": 0}},
		{"no glide", RecipeParams{"gen1_kick_pe_amt": 0}},
		{"click OFF", RecipeParams{"gen1_kick_click": 0}},
		{"only fundamental", RecipeParams{"gen1_kick_h2": 0, "gen1_kick_h3": 0, "gen1_kick_h4": 0, "gen1_kick_mode_gain": 0, "gen1_kick_click": 0, "gen1_kick_noise": 0, "gen1_kick_sat": 0, "gen1_kick_reverb": 0}},
	}
	for _, v := range variants {
		seed := cloneSeed(acousticKickSeed, v.over)
		x := f32toF64(renderSeed(seed, sr, samples))
		loE, loMod := bandModulation(x, sr, 40, 120)    // low groan/warble
		midE, midMod := bandModulation(x, sr, 120, 600) // mid creak
		fmt.Printf("  %-26s low(40-120) e=%.4f mod=%.2f | mid(120-600) e=%.4f mod=%.2f | peaks=%s\n",
			v.name, loE, loMod, midE, midMod, tailPeaks(x, sr))
	}
}

// bandModulation returns the mean [lo,hi] band energy of the tail (50-350 ms)
// and its WARBLE — the coefficient of variation of the band energy DETRENDED by
// the window's total energy (so a smooth decay counts as 0; only relative
// warble/beating counts). High warble in a band = a creak/groan there.
func bandModulation(x []float64, sr int, lo, hi float64) (meanE, warble float64) {
	start := sr * 50 / 1000
	end := sr * 350 / 1000
	if end > len(x) {
		end = len(x)
	}
	win := sr * 15 / 1000
	var ratios, energies []float64
	for s := start; s+win <= end; s += win {
		be, te := 0.0, 0.0
		for f := 30.0; f <= 4000; f += 25 {
			m := goertzelMagnitude(x[s:s+win], f, sr)
			p := m * m
			te += p
			if f >= lo && f <= hi {
				be += p
			}
		}
		energies = append(energies, be)
		if te > 0 {
			ratios = append(ratios, be/te)
		}
	}
	if len(ratios) == 0 {
		return 0, 0
	}
	mean := 0.0
	for _, v := range energies {
		mean += v
	}
	mean /= float64(len(energies))
	rm := 0.0
	for _, v := range ratios {
		rm += v
	}
	rm /= float64(len(ratios))
	varr := 0.0
	for _, v := range ratios {
		varr += (v - rm) * (v - rm)
	}
	varr /= float64(len(ratios))
	if rm > 0 {
		warble = math.Sqrt(varr) / rm
	}
	return mean, warble
}

// tailPeaks returns the 3 strongest frequencies (30-1500 Hz) in the tail
// (100-350 ms) — sharp resonant peaks there are the creak's pitch.
func tailPeaks(x []float64, sr int) string {
	start := sr * 100 / 1000
	end := sr * 350 / 1000
	if end > len(x) {
		end = len(x)
	}
	seg := x[start:end]
	type fp struct {
		f, m float64
	}
	var best []fp
	for f := 30.0; f <= 1500; f += 10 {
		m := goertzelMagnitude(seg, f, sr)
		best = append(best, fp{f, m})
	}
	// simple top-3 by magnitude
	for i := 0; i < 3; i++ {
		for j := i + 1; j < len(best); j++ {
			if best[j].m > best[i].m {
				best[i], best[j] = best[j], best[i]
			}
		}
	}
	return fmt.Sprintf("%.0f,%.0f,%.0f Hz", best[0].f, best[1].f, best[2].f)
}
