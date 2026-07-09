//go:build !test && !js

package main

import (
	"fmt"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// TestSnareGap analyzes the reference snare vs the current synth snare on the
// snare-specific axes (tone/noise split, body tuning, body vs buzz decay, attack)
// plus the shared kick metrics, to drive the tuning.
// go test -run TestSnareGap -v ./cmd/synth-match/
func TestSnareGap(t *testing.T) {
	const root = "/home/ymolinar/Repos/beatmo/"
	const refFile = "579499__yenus__fat-snare-bottom-ramon.wav"
	audio.Reset()
	audio.ResetInstruments()

	pcm, sr, err := audio.DecodeWAVToPCM(root + refFile)
	if err != nil {
		t.Fatalf("decode ref: %v", err)
	}
	refWave := wave.Wave{Samples: f32ToF64(pcm), SampleRate: sr}
	refSM := fingerprint.AnalyzeSnare(refWave)
	refK := fingerprint.KickAnalyze(refWave)

	buf, ssr := audio.RenderInstrumentOneShotRaw("snare")
	synWave := wave.Wave{Samples: f32ToF64(buf), SampleRate: ssr}
	synSM := fingerprint.AnalyzeSnare(synWave)
	synK := fingerprint.KickAnalyze(synWave)

	fmt.Printf("\n══ SNARE  ref=%s (%d smp, %.2fs) | synth (%d smp)\n", refFile, len(refWave.Samples), float64(len(refWave.Samples))/float64(sr), len(buf))
	row := func(n string, r, s float64) { fmt.Printf("   %-16s ref=%9.3f  syn=%9.3f\n", n, r, s) }
	row("bodyFundHz", refSM.BodyFundHz, synSM.BodyFundHz)
	row("toneNoiseRatio", refSM.ToneNoiseRatio, synSM.ToneNoiseRatio)
	row("lowBodyFrac", refSM.LowBodyFrac, synSM.LowBodyFrac)
	row("highBuzzFrac", refSM.HighBuzzFrac, synSM.HighBuzzFrac)
	row("bodyDecayMs", refSM.BodyDecayMs, synSM.BodyDecayMs)
	row("buzzDecayMs", refSM.BuzzDecayMs, synSM.BuzzDecayMs)
	row("attackMs", refSM.AttackMs, synSM.AttackMs)
	row("centroidHz", refSM.CentroidHz, synSM.CentroidHz)
	row("flatness", refSM.Flatness, synSM.Flatness)
	fmt.Printf("   -- shared kick metrics --\n")
	row("k.flatnessAvg", refK.FlatnessAvg, synK.FlatnessAvg)
	row("k.centroidMean", meanTrace(refK.CentroidTrace), meanTrace(synK.CentroidTrace))
	row("k.crest", refK.Crest, synK.Crest)
	row("k.tailDurSec", refK.TailDurationSec, synK.TailDurationSec)
}

func meanTrace(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := 0.0
	for _, v := range xs {
		s += v
	}
	return s / float64(len(xs))
}
