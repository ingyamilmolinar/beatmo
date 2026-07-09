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

// TestSaxProbe is an exploratory dump of EVERY fingerprint metric for the
// baritone-sax reference vs the current sax synth, over BOTH the note-onset
// window (attack behavior) and the loudest sustain window (steady timbre).
// It exists to expose which axes the shipped wind-gap rows miss.
// Run: go test -run TestSaxProbe -v ./cmd/synth-match/
func TestSaxProbe(t *testing.T) {
	const root = "/home/ymolinar/Repos/beatmo/"
	audio.Reset()
	audio.ResetInstruments()

	pcm, sr, err := audio.DecodeWAVToPCM(root + "360251__mtg__sax-baritone-c3.wav")
	if err != nil {
		t.Fatal(err)
	}
	refW := wave.Wave{Samples: f32ToF64(pcm), SampleRate: sr}

	refSus := fingerprint.AutoSegment(refW, fingerprint.SustainLenSec)
	f0 := fingerprint.DetectF0(refSus)
	pitch := 12 * math.Log2(f0/220.0)
	fmt.Printf("ref: %d samples @ %d Hz (%.2fs)  f0=%.2f Hz  pitch=%+.2f st\n",
		len(refW.Samples), sr, float64(len(refW.Samples))/float64(sr), f0, pitch)

	synW, err := synthmatch.RenderInstrument("sax", pitch, sr, 2.0)
	if err != nil {
		t.Fatal(err)
	}

	dump := func(name string, w wave.Wave) {
		sus := fingerprint.AutoSegment(w, fingerprint.SustainLenSec)
		fpSus := fingerprint.FromWave(sus, name+"-sustain")
		wa := fingerprint.ComputeWindAttack(w)

		fmt.Printf("\n════ %s ════\n", name)
		fmt.Printf(" ATTACK onset=%.2fs rise10-90=%.3fs tempCentroid=%.2fs\n",
			wa.OnsetSec, wa.AttackTimeSec, wa.TemporalCentroid)
		fmt.Printf("   centroid@25/50/100/300ms: %.0f %.0f %.0f %.0f  slope=%.0f Hz/s\n",
			wa.AttackCentroids[0], wa.AttackCentroids[1], wa.AttackCentroids[2], wa.AttackCentroids[3], wa.CentroidSlopeHzS)
		fmt.Printf("   irregularity=%.2f dB  formantBands=%v\n",
			fingerprint.SpectralIrregularity(fpSus.Partials),
			fmtBands(fingerprint.FormantBandEnergies(sus, fingerprint.WindFormantBandsHz)))
		fmt.Printf(" SUSTAIN f0=%.2f centroid=%.0f rolloff=%.0f evenOdd=%.3f inharm=%.4f noise=%.3f HNR=%.1f\n",
			fpSus.F0Hz, fpSus.SpectralCentroid, fpSus.SpectralRolloff, fpSus.EvenOddRatio, fpSus.Inharmonicity, fpSus.NoiseRatio, fpSus.HNR)
		fmt.Printf("   flat=%.4f crest=%.1f skew=%.2f kurt=%.2f  tristim=[%.3f %.3f %.3f]\n",
			fpSus.SpectralFlatness, fpSus.SpectralCrest, fpSus.SpectralSkewness, fpSus.SpectralKurtosis,
			fpSus.Tristimulus[0], fpSus.Tristimulus[1], fpSus.Tristimulus[2])
		fmt.Printf("   partials: %s\n", normProfile(fpSus.Partials))
		fmt.Printf("   partials9-16: ")
		base := fpSus.Partials[0]
		if base <= 1e-9 {
			base = 1
		}
		for i := 8; i < 16; i++ {
			fmt.Printf("%.3f ", fpSus.Partials[i]/base)
		}
		fmt.Println()
		if v := fpSus.Vibrato; v != nil {
			fmt.Printf("   vibrato: rate=%.2fHz extent=%.2f¢ jitter=%.3f AMdepth=%.4f onset=%.2fs detected=%v\n",
				v.RateHz, v.ExtentCents, v.Jitter, v.AMDepth, v.OnsetSec, v.Detected)
		}
		if np := fpSus.NoiseProf; np != nil {
			fmt.Printf("   noiseProf: residRatio=%.3f residCentroid=%.0fHz bands=%v\n",
				np.ResidualRatio, np.ResidualCentroidHz, fmtBands(np.ResidualBandLevels))
		}
		if c := fpSus.Coupling; c != nil {
			fmt.Printf("   coupling: cRMScorr=%.3f brModDepth=%.4f brModRate=%.2fHz macro=%.3f\n",
				c.CentroidRMSCorr, c.BrightnessModDepth, c.BrightnessModRateHz, c.MacroCentroidRMSCorr)
		}
		if se := fpSus.SpectralEnv; se != nil {
			fmt.Printf("   formants:")
			for i, f := range se.Formants {
				if i >= 5 {
					break
				}
				fmt.Printf(" [%.0fHz bw=%.0f %+.1fdB]", f.Hz, f.BandwidthHz, f.GainDB)
			}
			fmt.Println()
		}
		if h := fpSus.Harmonics; h != nil {
			fmt.Printf("   harmSustain(1-8):")
			for i := 0; i < 8 && i < h.N; i++ {
				fmt.Printf(" %.2f", h.SustainLevel[i])
			}
			fmt.Printf("\n   harmDecay dB/s(1-8):")
			for i := 0; i < 8 && i < h.N; i++ {
				fmt.Printf(" %.0f", h.DecayRate[i])
			}
			eo := h.EvenOddOverTime
			if len(eo) > 4 {
				fmt.Printf("\n   evenOdd t0=%.2f t25=%.2f t50=%.2f t75=%.2f t100=%.2f",
					eo[0], eo[len(eo)/4], eo[len(eo)/2], eo[3*len(eo)/4], eo[len(eo)-1])
			}
			fmt.Println()
		}
	}

	dump("REF", refW)
	dump("SYN", synW)
}

func fmtBands(b []float64) []string {
	out := make([]string, len(b))
	for i, v := range b {
		out[i] = fmt.Sprintf("%.3f", v)
	}
	return out
}
