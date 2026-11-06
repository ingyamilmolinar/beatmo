//go:build !js

package main

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wavgen"
)

// generatedD5Path writes a GENERATED bowed-string-like D5 note into t.TempDir()
// and returns its path: a 587.33 Hz harmonic tone (1/k partials) with a
// 5.5 Hz, ±20-cent vibrato and an attack/sustain/release envelope. Tests never
// read reference recordings -- those are local tuning material only (see
// internal/wavref); everything here must run identically on any checkout.
func generatedD5Path(t *testing.T) string {
	t.Helper()
	const (
		sr       = 44100
		f0       = 587.33 // D5
		vibHz    = 5.5
		vibCents = 20.0
		attack   = 0.05
		sustain  = 2.0
		release  = 0.3
	)
	total := attack + sustain + release
	n := int(total * sr)
	pcm := make([]float32, n)
	phase := 0.0
	for i := range pcm {
		ts := float64(i) / sr
		f := f0 * math.Pow(2, vibCents/1200*math.Sin(2*math.Pi*vibHz*ts))
		phase += 2 * math.Pi * f / sr
		var s float64
		for k := 1; k <= 6; k++ {
			s += math.Sin(float64(k)*phase) / float64(k)
		}
		env := 1.0
		switch {
		case ts < attack:
			env = ts / attack
		case ts > attack+sustain:
			env = math.Max(0, 1-(ts-attack-sustain)/release)
		}
		pcm[i] = float32(0.4 * env * s)
	}
	data, err := wavgen.EncodeWAV(pcm, sr)
	if err != nil {
		t.Fatalf("encode generated D5: %v", err)
	}
	p := filepath.Join(t.TempDir(), "violin-d5.wav")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatalf("write generated D5: %v", err)
	}
	return p
}
