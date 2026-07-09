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

// TestWindGap prints, for each wind instrument, the SYNTH fingerprint next to its
// REFERENCE fingerprint on the identity-defining axes (even/odd, noise, centroid,
// attack, vibrato, harmonic profile). The tuning loop: run → read the gap → edit
// the seed (or C DSP) → rebuild → run again → confirm the synth moved toward the
// reference. Run: go test -run TestWindGap -v ./cmd/synth-match/
func TestWindGap(t *testing.T) {
	const root = "/home/ymolinar/Repos/beatmo/"
	audio.Reset()
	audio.ResetInstruments()

	cases := []struct {
		id, file string
		f0       float64
	}{
		{"oboe", "22686__acclivity__oboe_a_440.wav", 440.00},
		{"flute", "373313__sgossner__flute-expressive-sustain-e5-ldflute_expvib_e4_v1_1.wav", 659.25},
		{"trumpet", "374162__sgossner__trumpet-sustain-f4-sum_shtrumpet_sus_g3_v3_rr1.wav", 349.23},
		{"french-horn", "682231__henkonen__fhorn-18.wav", 262.9},
		// The bari file is named c3 but MEASURES 122.8 Hz (≈B2); its fundamental
		// sits ~7 dB below H2, so pre-octave-guard-fix analyses ran at 245.6 Hz.
		{"sax", "360251__mtg__sax-baritone-c3.wav", 122.8},
	}

	for _, c := range cases {
		pcm, sr, err := audio.DecodeWAVToPCM(root + c.file)
		if err != nil {
			t.Logf("skip %s: %v", c.file, err)
			continue
		}
		refFP := fingerprint.FromWave(fingerprint.AutoSegment(wave.Wave{Samples: f32ToF64(pcm), SampleRate: sr}, fingerprint.SustainLenSec), "ref")
		pitch := 12 * math.Log2(c.f0/220.0)
		cw, err := synthmatch.RenderInstrument(c.id, pitch, sr, 2.0)
		if err != nil {
			t.Logf("render %s: %v", c.id, err)
			continue
		}
		synFP := fingerprint.FromWave(fingerprint.AutoSegment(cw, fingerprint.SustainLenSec), "syn")
		fmt.Printf("\n══ %-12s (%s)\n", c.id, c.file)
		row := func(name string, r, s float64) {
			fmt.Printf("   %-14s ref=%8.3f  syn=%8.3f\n", name, r, s)
		}
		row("evenOdd", refFP.EvenOddRatio, synFP.EvenOddRatio)
		row("noiseRatio", refFP.NoiseRatio, synFP.NoiseRatio)
		row("centroidHz", refFP.SpectralCentroid, synFP.SpectralCentroid)
		// attackSec (sustain-window) is unreliable for winds — AutoSegment lands
		// mid-note. rise10-90 below is the onset-aligned attack (wind_metrics.go).
		row("attackSec", refFP.AttackTimeSec, synFP.AttackTimeSec)
		row("flatness", refFP.SpectralFlatness, synFP.SpectralFlatness)
		refWA := fingerprint.ComputeWindAttack(wave.Wave{Samples: f32ToF64(pcm), SampleRate: sr})
		synWA := fingerprint.ComputeWindAttack(cw)
		row("rise10-90", refWA.AttackTimeSec, synWA.AttackTimeSec)
		row("atkCenSlope", refWA.CentroidSlopeHzS, synWA.CentroidSlopeHzS)
		row("irregularity", fingerprint.SpectralIrregularity(refFP.Partials), fingerprint.SpectralIrregularity(synFP.Partials))
		refBands := fingerprint.FormantBandEnergies(fingerprint.AutoSegment(wave.Wave{Samples: f32ToF64(pcm), SampleRate: sr}, fingerprint.SustainLenSec), fingerprint.WindFormantBandsHz)
		synBands := fingerprint.FormantBandEnergies(fingerprint.AutoSegment(cw, fingerprint.SustainLenSec), fingerprint.WindFormantBandsHz)
		fmt.Printf("   formantBands   ref=%v\n                  syn=%v\n", refBands, synBands)
		if refFP.Vibrato != nil && synFP.Vibrato != nil {
			row("vibratoHz", refFP.Vibrato.RateHz, synFP.Vibrato.RateHz)
			row("vibratoCents", refFP.Vibrato.ExtentCents, synFP.Vibrato.ExtentCents)
		}
		fmt.Printf("   partials(norm) ref=%s\n                  syn=%s\n", normProfile(refFP.Partials), normProfile(synFP.Partials))
	}
}

// normProfile prints partials 1..8 normalized so P1=1 (the harmonic-envelope shape).
func normProfile(p [16]float64) string {
	base := p[0]
	if base <= 1e-9 {
		for _, v := range p {
			if v > base {
				base = v
			}
		}
	}
	if base <= 1e-9 {
		base = 1
	}
	s := ""
	for i := 0; i < 8; i++ {
		s += fmt.Sprintf("%.2f ", p[i]/base)
	}
	return s
}
