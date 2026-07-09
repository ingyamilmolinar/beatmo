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

// TestStringGap: synth-vs-ref identity axes for the non-wind melodic refs
// (bowed strings, plucked harp, piano) to find which are structurally off.
// go test -run TestStringGap -v ./cmd/synth-match/
func TestStringGap(t *testing.T) {
	const root = "/home/ymolinar/Repos/beatmo/"
	audio.Reset()
	audio.ResetInstruments()

	cases := []struct {
		id, file string
		f0       float64
	}{
		{"cello", "257996__xserra__cello-d2.wav", 73.42},
		{"violin", "356181__mtg__violin-d5.wav", 587.33},
		{"harp", "127160__daphne_in_wonderland__celtic_harp_g2.wav", 98.00},
		{"piano-grand", "68448__pinkyfinger__piano-g.wav", 0},
	}
	for _, c := range cases {
		pcm, sr, err := audio.DecodeWAVToPCM(root + c.file)
		if err != nil {
			t.Logf("skip %s: %v", c.file, err)
			continue
		}
		seg := fingerprint.AutoSegment(wave.Wave{Samples: f32ToF64(pcm), SampleRate: sr}, fingerprint.SustainLenSec)
		refFP := fingerprint.FromWave(seg, "ref")
		f0 := c.f0
		if f0 <= 0 {
			f0 = fingerprint.DetectF0(seg)
		}
		pitch := 12 * math.Log2(f0/220.0)
		cw, err := synthmatch.RenderInstrument(c.id, pitch, sr, 2.0)
		if err != nil {
			t.Logf("render %s: %v", c.id, err)
			continue
		}
		synFP := fingerprint.FromWave(fingerprint.AutoSegment(cw, fingerprint.SustainLenSec), "syn")
		fmt.Printf("\n══ %-12s (%s, f0=%.0f)\n", c.id, c.file, f0)
		row := func(n string, r, s float64) { fmt.Printf("   %-13s ref=%8.3f  syn=%8.3f\n", n, r, s) }
		row("evenOdd", refFP.EvenOddRatio, synFP.EvenOddRatio)
		row("noiseRatio", refFP.NoiseRatio, synFP.NoiseRatio)
		row("centroidHz", refFP.SpectralCentroid, synFP.SpectralCentroid)
		row("inharmonic", refFP.Inharmonicity, synFP.Inharmonicity)
		row("attackSec", refFP.AttackTimeSec, synFP.AttackTimeSec)
		row("decaySlope", refFP.DecaySlope, synFP.DecaySlope)
		if refFP.Vibrato != nil && synFP.Vibrato != nil {
			row("vibratoHz", refFP.Vibrato.RateHz, synFP.Vibrato.RateHz)
			row("vibratoCents", refFP.Vibrato.ExtentCents, synFP.Vibrato.ExtentCents)
		}
		fmt.Printf("   partials ref=%s\n            syn=%s\n", normProfile(refFP.Partials), normProfile(synFP.Partials))
	}
}
