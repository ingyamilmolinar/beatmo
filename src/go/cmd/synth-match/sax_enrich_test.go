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

// TestSaxEnrich sweeps the now-config-exposed sax reed params (P2) to find a
// richer harmonic profile — the reference is rich (P2 0.51/P3 0.69/P5 0.38); the
// shipped default is dull (P2 0.21/P3 0.32/P5 0.12).
// go test -run TestSaxEnrich -v ./cmd/synth-match/
func TestSaxEnrich(t *testing.T) {
	const root = "/home/ymolinar/Repos/beatmo/"
	audio.Reset()
	audio.ResetInstruments()

	pcm, sr, err := audio.DecodeWAVToPCM(root + "360251__mtg__sax-baritone-c3.wav")
	if err != nil {
		t.Fatal(err)
	}
	seg := fingerprint.AutoSegment(wave.Wave{Samples: f32ToF64(pcm), SampleRate: sr}, fingerprint.SustainLenSec)
	refFP := fingerprint.FromWave(seg, "ref")
	f0 := fingerprint.DetectF0(seg)
	pitch := 12 * math.Log2(f0/220.0)
	fmt.Printf("ref f0=%.1f pitch=%.1f  evenOdd=%.2f centroid=%.0f  profile=%s\n",
		f0, pitch, refFP.EvenOddRatio, refFP.SpectralCentroid, normProfile(refFP.Partials))

	configs := []struct {
		name                                   string
		blow, roff, rslope, refl, breath, loss float64
	}{
		{"default", 0.15, 0.58, 0.28, -0.94, 0.85, 0.70},
		{"blow18", 0.18, 0.58, 0.30, -0.94, 0.85, 0.72},
		{"blow20", 0.20, 0.58, 0.30, -0.94, 0.85, 0.72},
		{"blow22", 0.22, 0.58, 0.32, -0.94, 0.90, 0.74},
		{"blow24", 0.24, 0.56, 0.32, -0.94, 0.90, 0.74},
		{"blow22-soft", 0.22, 0.58, 0.30, -0.94, 0.88, 0.72},
	}
	for _, c := range configs {
		ov := map[string]float64{
			"osc_sax_blow": c.blow, "osc_sax_reed_off": c.roff, "osc_sax_reed_slope": c.rslope,
			"osc_sax_reflect": c.refl, "osc_sax_breath": c.breath, "osc_sax_loss": c.loss,
		}
		cw, err := synthmatch.RenderWithParams("sax", pitch, sr, 2.0, ov)
		if err != nil {
			t.Logf("%s: %v", c.name, err)
			continue
		}
		s := fingerprint.FromWave(fingerprint.AutoSegment(cw, fingerprint.SustainLenSec), "s")
		fmt.Printf("%-16s evenOdd=%.2f centroid=%.0f flat=%.3f noise=%.2f  profile=%s\n",
			c.name, s.EvenOddRatio, s.SpectralCentroid, s.SpectralFlatness, s.NoiseRatio, normProfile(s.Partials))
	}
}
