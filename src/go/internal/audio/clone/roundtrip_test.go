//go:build !test

package clone

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// TestCloneRoundTripThroughTheEngine: a known modal document → the REAL
// render (C engine, document FX, normalisation) → Prepare → Analyze → Invert
// → render again. The second document must recover the first's modes and
// sound like it.
func TestCloneRoundTripThroughTheEngine(t *testing.T) {
	const rate = 48000
	truth := eightModes()
	for i := range truth {
		truth[i].Amp *= 0.4
		truth[i].Alpha *= 3 // rings out inside the render: a clip cut mid-ring is a fade no mode can model
	}
	a0 := analysisOf(truth, PitchInfo{Pitched: true, F0Hz: 261.63})
	a0.Residual.Bands[0].LevelDB, a0.Residual.Bands[1].LevelDB = -40, -46
	d0, rep0, err := Invert(a0, InvertOptions{ID: "roundtrip"})
	if err != nil {
		t.Fatal(err)
	}
	x := audio.RenderAsShipped(d0, rate, rep0.FitPitch)
	pre := make([]float64, rate/20) // 50 ms of silence before the strike
	for i := range pre {
		pre[i] = 1e-6 * math.Sin(float64(i)) // a floor Prepare can measure
	}
	p, err := Prepare(append(pre, x...), rate, PrepareOptions{F0Hz: 261.63, Label: "roundtrip"})
	if err != nil {
		t.Fatal(err)
	}
	a, err := Analyze(p)
	if err != nil {
		t.Fatal(err)
	}
	d1, rep1, err := Invert(a, InvertOptions{ID: "roundtrip-clone"})
	if err != nil {
		t.Fatal(err)
	}
	d1.Render = d0.Render
	// The render is peak-normalised: compare shapes, scaled by the strongest mode.
	scale := 0.0
	for _, m := range a.Modes {
		if math.Abs(m.Freq-truth[0].Freq) < 1 {
			scale = m.Amp / truth[0].Amp
		}
	}
	if scale == 0 {
		t.Fatalf("fundamental not recovered: %v", a.Modes)
	}
	for _, w := range truth[:6] {
		var g *Mode
		for i := range a.Modes {
			if math.Abs(1200*math.Log2(a.Modes[i].Freq/w.Freq)) < 1 && (g == nil || a.Modes[i].Amp > g.Amp) {
				g = &a.Modes[i] // the strongest within a cent (noise can add a weak neighbour)
			}
		}
		if g == nil {
			t.Fatalf("mode %.1f Hz not recovered within 1 cent", w.Freq)
		}
		if math.Abs(g.Alpha/w.Alpha-1) > 0.10 || math.Abs(20*math.Log10(g.Amp/(scale*w.Amp))) > 0.5 {
			t.Errorf("mode %.1f Hz: α %.3f (want %.3f), A %.4g (want %.4g)", w.Freq, g.Alpha, w.Alpha, g.Amp, scale*w.Amp)
		}
	}
	y := audio.RenderAsShipped(d1, rate, rep1.FitPitch)
	ref := NewLossRef(p, 2)
	l := ref.Score(y, 0, LossWeights{MRSTFT: 1, Envelope: 1}, LossReport{})
	t.Logf("round trip: %d modes, log-mag %.3f dB, SC %.4f, envelope %.3f dB", len(a.Modes), l.LogMagDB, l.SC, l.Envelope)
	if l.LogMagDB > 1.5 || l.SC > 0.05 || l.Envelope > 0.5 {
		t.Fatalf("the clone of a clone does not sound like it: %+v", l)
	}
}
