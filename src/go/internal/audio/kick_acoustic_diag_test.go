//go:build !test && !js

package audio

import (
	"fmt"
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// TestKickAcousticDiagnose prints the FULL timbre fingerprint (spectral shape,
// MFCC, HNR, inharmonicity, attack) for the synth acoustic vs the reference —
// the metrics the KICK distance currently ignores — to find WHY it sounds wrong
// despite matching bands/crest/settle. Diagnostic only (always passes).
func TestKickAcousticDiagnose(t *testing.T) {
	const sr = 44100
	ref := decodeRefAt(t, "../../../../36010__sandyrb__dnb-kick-003.wav", sr)
	syn := peakNorm(f32toF64(func() []float32 { b, _ := RenderInstrumentOneShotRaw("kick-acoustic"); return b }()))

	rw := wave.Wave{Samples: ref, SampleRate: sr}
	sw := wave.Wave{Samples: syn, SampleRate: sr}
	rf := fingerprint.FromWave(rw, "ref")
	sf := fingerprint.FromWave(sw, "synth")

	row := func(name string, r, s float64) {
		fmt.Printf("  %-18s ref=%-10.3f synth=%-10.3f  Δ=%+.3f\n", name, r, s, s-r)
	}
	fmt.Println("── ACOUSTIC KICK: full-fingerprint divergence (ref vs synth) ──")
	row("F0Hz", rf.F0Hz, sf.F0Hz)
	row("SpectralCentroid", rf.SpectralCentroid, sf.SpectralCentroid)
	row("SpectralRolloff", rf.SpectralRolloff, sf.SpectralRolloff)
	row("SpectralFlatness", rf.SpectralFlatness, sf.SpectralFlatness)
	row("SpectralCrest", rf.SpectralCrest, sf.SpectralCrest)
	row("Inharmonicity", rf.Inharmonicity, sf.Inharmonicity)
	row("HNR", rf.HNR, sf.HNR)
	row("AttackTimeSec", rf.AttackTimeSec, sf.AttackTimeSec)
	row("LogAttackTime", rf.LogAttackTime, sf.LogAttackTime)

	// MFCC distance (the timbre fingerprint the kick metric ignores).
	var mfccDist float64
	fmt.Printf("  MFCC ref  : ")
	for i := 0; i < 13; i++ {
		fmt.Printf("%.1f ", rf.MFCC[i])
	}
	fmt.Printf("\n  MFCC synth: ")
	for i := 0; i < 13; i++ {
		fmt.Printf("%.1f ", sf.MFCC[i])
		d := rf.MFCC[i] - sf.MFCC[i]
		mfccDist += d * d
	}
	fmt.Printf("\n  MFCC L2 distance = %.2f\n", math.Sqrt(mfccDist))

	// Attack-window spectral centroid (the beater-click brightness) — computed on
	// the first 20 ms only, where a real beater and a synth click differ most.
	aw := int(0.02 * sr)
	fmt.Printf("  attack(20ms) centroid: ref=%.0f Hz  synth=%.0f Hz\n",
		windowCentroid(ref, sr, aw), windowCentroid(syn, sr, aw))
	fmt.Printf("  full Fingerprint.Distance = %.3f\n", fingerprint.Distance(rf, sf).Total)
}

// windowCentroid returns the spectral centroid (Hz) of the first n samples.
func windowCentroid(x []float64, sr, n int) float64 {
	if n > len(x) {
		n = len(x)
	}
	num, den := 0.0, 0.0
	for f := 50.0; f <= 8000; f += 25 {
		m := goertzelMagnitude(x[:n], f, sr)
		num += f * m
		den += m
	}
	if den == 0 {
		return 0
	}
	return num / den
}
