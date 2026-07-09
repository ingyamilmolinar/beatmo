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

// TestPianoAnalyze prints the piano-specific metrics (inharmonicity B, two-stage
// decay, hammer attack, per-partial stretch) for the reference piano vs the synth
// piano-grand, to drive making the synth sound less electronic.
// go test -run TestPianoAnalyze -v ./cmd/synth-match/
func TestPianoAnalyze(t *testing.T) {
	const root = "/home/ymolinar/Repos/beatmo/"
	const refFile = "68448__pinkyfinger__piano-g.wav"
	audio.Reset()
	audio.ResetInstruments()

	pcm, sr, err := audio.DecodeWAVToPCM(root + refFile)
	if err != nil {
		t.Fatalf("decode ref: %v", err)
	}
	refWave := wave.Wave{Samples: f32ToF64(pcm), SampleRate: sr}
	f0 := fingerprint.DetectF0(fingerprint.AutoSegment(refWave, fingerprint.SustainLenSec))
	refPM := fingerprint.AnalyzePiano(refWave, f0)

	pitch := 12 * math.Log2(f0/220.0)
	cw, err := synthmatch.RenderInstrument("piano-grand", pitch, sr, 2.5)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	synPM := fingerprint.AnalyzePiano(cw, f0)

	row := func(n string, r, s float64) { fmt.Printf("   %-16s ref=%9.4f  syn=%9.4f\n", n, r, s) }
	fmt.Printf("\n══ PIANO  ref=%s  f0=%.1f Hz\n", refFile, f0)
	row("inharmonicB", refPM.InharmonicityB, synPM.InharmonicityB)
	row("attackMs", refPM.AttackMs, synPM.AttackMs)
	row("decayFast dB/s", refPM.DecayFastDBs, synPM.DecayFastDBs)
	row("decaySlow dB/s", refPM.DecaySlowDBs, synPM.DecaySlowDBs)
	row("twoStageRatio", refPM.TwoStageRatio, synPM.TwoStageRatio)
	row("centroidHz", refPM.CentroidHz, synPM.CentroidHz)
	fmt.Printf("   partial stretch (cents, n=2..8):\n     ref: %s\n     syn: %s\n",
		fmtStretch(refPM.StretchCents), fmtStretch(synPM.StretchCents))
}

func fmtStretch(s []float64) string {
	out := ""
	for n := 2; n <= 8 && n-1 < len(s); n++ {
		out += fmt.Sprintf("H%d=%+.0f ", n, s[n-1])
	}
	return out
}
