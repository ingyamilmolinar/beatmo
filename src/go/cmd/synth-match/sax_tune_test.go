//go:build !test && !js

package main

import (
	"fmt"
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/audio/synthmatch"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// TestSaxTune sweeps candidate saxSeed reworks (as param overrides on the
// shipped seed) against the TRUE-grid reference measurements of
// 360251__mtg__sax-baritone-c3.wav (f0 ≈ 122.8 Hz — the old analysis was an
// octave up; see DetectF0's odd-harmonic evidence fix). Prints one compact gap
// row per config. Run: go test -run TestSaxTune -v ./cmd/synth-match/
func TestSaxTune(t *testing.T) {
	const root = "/home/ymolinar/Repos/beatmo/"
	audio.Reset()
	audio.ResetInstruments()

	pcm, sr, err := audio.DecodeWAVToPCM(root + "360251__mtg__sax-baritone-c3.wav")
	if err != nil {
		t.Fatal(err)
	}
	refW := wave.Wave{Samples: f32ToF64(pcm), SampleRate: sr}

	// Fixed-grid harmonic analysis at the reference's true f0: YIN on
	// intermediate candidate renders can lock onto H3 (saw+BP slots), which
	// would silently switch the partial grid mid-sweep. Same math as
	// computeSpectral but with the grid pinned.
	const gridF0 = 122.8
	fixedGrid := func(sus wave.Wave) (partials [16]float64, evenOdd, noise float64) {
		mag, binHz := wave.MagnitudeSpectrum(sus, 16384, wave.WindowHann)
		total, harm := 0.0, 0.0
		for _, m := range mag {
			total += m * m
		}
		var even, odd float64
		for k := 1; float64(k)*gridF0 < float64(sus.SampleRate)/2; k++ {
			bin := int(gridF0*float64(k)/binHz + 0.5)
			best := 0.0
			for b := bin - 2; b <= bin+2; b++ {
				if b >= 0 && b < len(mag) && mag[b] > best {
					best = mag[b]
				}
			}
			if k <= 16 {
				partials[k-1] = best
			}
			harm += best * best
			if k%2 == 0 {
				even += best * best
			} else {
				odd += best * best
			}
		}
		if odd > 0 {
			evenOdd = even / odd
		}
		if total > 0 {
			noise = math.Max(0, 1-harm/total)
		}
		return
	}

	report := func(name string, w wave.Wave) {
		sus := fingerprint.AutoSegment(w, fingerprint.SustainLenSec)
		fp := fingerprint.FromWave(sus, name)
		wa := fingerprint.ComputeWindAttack(w)
		partials, evenOdd, noise := fixedGrid(sus)
		base := partials[1] // rel-H2 (the ref's dominant partial)
		if base < 1e-9 {
			base = 1
		}
		hi := 0.0
		for i := 8; i < 16; i++ {
			hi += partials[i] / base
		}
		vib := 0.0
		if fp.Vibrato != nil {
			vib = fp.Vibrato.ExtentCents
		}
		coup := 0.0
		if fp.Coupling != nil {
			coup = fp.Coupling.CentroidRMSCorr
		}
		fmt.Printf("%-14s f0=%5.1f eo=%.2f cen=%4.0f noi=%.3f irr=%.2f hi916=%.2f rise=%.3f vib=%.1f coup=%+.2f\n",
			name, fp.F0Hz, evenOdd, fp.SpectralCentroid, noise,
			fingerprint.SpectralIrregularity(partials), hi,
			wa.AttackTimeSec, vib, coup)
		relP := ""
		for i := 0; i < 8; i++ {
			relP += fmt.Sprintf("%.2f ", partials[i]/base)
		}
		rel916 := ""
		for i := 8; i < 16; i++ {
			rel916 += fmt.Sprintf("%.2f ", partials[i]/base)
		}
		fmt.Printf("%14s P1-8(relH2): %s| P9-16: %s bands=%v\n", "",
			relP, rel916,
			fmtBands(fingerprint.FormantBandEnergies(sus, fingerprint.WindFormantBandsHz)))
	}

	refSeg := fingerprint.AutoSegment(refW, fingerprint.SustainLenSec)
	f0 := fingerprint.DetectF0(refSeg)
	pitch := 12 * math.Log2(f0/220.0)
	fmt.Printf("f0=%.2f pitch=%.2f\n", f0, pitch)
	report("REF", refW)

	// base rebuild: sines pin H1-H4 + H6 (true-grid profile, rel H2=1.0 scale
	// ×0.5), saw+BP slots build the 820 Hz knee fill and the 2.1 kHz formant
	// plateau, breath noise moves UP to the ref's residual centroid (~1.5 kHz).
	rebuild := map[string]float64{
		// sines: H1 weak, H2 dominant, H3 near-equal, H4 half, H6 formant bump
		"gen2_freq": 1.0, "gen2_gain": 0.22,
		"gen3_freq": 2.0, "gen3_gain": 0.50,
		"gen4_freq": 3.0, "gen4_gain": 0.45,
		"gen5_freq": 4.0, "gen5_gain": 0.37,
		"gen6_freq": 6.0, "gen6_gain": 0.47,
		// saw through BP @ 820 (fills H5/H7-H10 with the knee shape); Q 1 leaked
		// 32% of its energy into 0-450 Hz, muddying the pinned sines — focus it
		"gen7_source": 1, "gen7_wave": 1, "gen7_freq_mode": 0, "gen7_freq": 1.0,
		"gen7_gain": 1.6, "gen7_filt_type": 5, "gen7_filt_freq": 820, "gen7_filt_q": 1.8,
		// saw through BP @ 2100 (the H14-H18 plateau / F2+F3); Q 1 was flat
		"gen8_source": 1, "gen8_wave": 1, "gen8_freq_mode": 0, "gen8_freq": 1.0,
		"gen8_gain": 4.0, "gen8_filt_type": 5, "gen8_filt_freq": 2050, "gen8_filt_q": 2.0,
		// breath noise at the ref's residual centroid (~1.5 kHz); Q 0.5 sprayed
		// energy to 2-8 kHz (centroid 3191 vs ref 1489) — narrow + quiet
		"gen1_gain": 0.18, "gen1_filt_freq": 1200, "gen1_filt_q": 2.5,
		// slower reedy onset (ref rise10-90 = 0.22 s; 0.25 measured only 0.123)
		"amp_attack": 0.5,
		// ref is nearly straight (2.4¢, undetected)
		"lfo_depth":     0.002,
		"filter_cutoff": 2800, // open the master LP — brightness comes from the BPs now
	}

	mod := func(over map[string]float64) map[string]float64 {
		m := make(map[string]float64, len(rebuild)+len(over))
		for k, v := range rebuild {
			m[k] = v
		}
		for k, v := range over {
			m[k] = v
		}
		return m
	}

	configs := []struct {
		name string
		over map[string]float64
	}{
		{"final", mod(map[string]float64{
			"amp_decay": 2.5, "amp_sustain": 0.45, "filter_cutoff": 2400,
			"gen7_gain": 1.9, "gen6_gain": 0.53})},
		{"final-lp2200", mod(map[string]float64{
			"amp_decay": 2.5, "amp_sustain": 0.45, "filter_cutoff": 2200,
			"gen7_gain": 1.9, "gen6_gain": 0.53, "gen8_gain": 4.6})},
		{"final-res", mod(map[string]float64{
			"amp_decay": 2.5, "amp_sustain": 0.45, "filter_cutoff": 2400,
			"gen7_gain": 1.9, "gen6_gain": 0.53, "filter_resonance": 1.6})},
	}
	for _, c := range configs {
		var cw wave.Wave
		var err error
		if c.over == nil {
			cw, err = synthmatch.RenderInstrument("sax", pitch, sr, 2.0)
		} else {
			cw, err = synthmatch.RenderWithParams("sax", pitch, sr, 2.0, c.over)
		}
		if err != nil {
			t.Logf("%s: %v", c.name, err)
			continue
		}
		report(c.name, cw)
	}
}
