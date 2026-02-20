//go:build test || js

package audio

import "math"

// gate implements a noise gate with envelope-follower-based gain control.
// Signals below the threshold are attenuated by the range parameter;
// signals above pass through at unity. Attack and release control the
// envelope follower smoothing, mirroring the compressor's approach.
type gate struct {
	thresholdDB float64 // -60 to 0 dB
	attackMs    float64 // 0.1 to 50 ms
	releaseMs   float64 // 1 to 500 ms
	rangeDB     float64 // -90 to 0 dB

	sr           int
	envelope     float64
	attackCoef   float64
	releaseCoef  float64
	thresholdLin float64
	rangeLin     float64
}

func newGate(sr int, params map[string]float64) *gate {
	g := &gate{sr: sr}
	g.thresholdDB = clampf(params["threshold"], -60, 0)
	g.attackMs = clampf(params["attack"], 0.1, 50)
	g.releaseMs = clampf(params["release"], 1, 500)
	g.rangeDB = clampf(params["range"], -90, 0)
	g.recalc()
	return g
}

func (g *gate) recalc() {
	sr := float64(g.sr)
	if sr <= 0 {
		sr = 44100
	}
	g.attackCoef = math.Exp(-1.0 / (g.attackMs * 0.001 * sr))
	g.releaseCoef = math.Exp(-1.0 / (g.releaseMs * 0.001 * sr))
	g.thresholdLin = math.Pow(10, g.thresholdDB/20)
	g.rangeLin = math.Pow(10, g.rangeDB/20)
}

func (g *gate) ProcessSample(x float64) float64 {
	// Peak envelope follower.
	absX := math.Abs(x)
	if absX > g.envelope {
		g.envelope = g.attackCoef*g.envelope + (1-g.attackCoef)*absX
	} else {
		g.envelope = g.releaseCoef*g.envelope + (1-g.releaseCoef)*absX
	}

	// Gate gain: unity above threshold, rangeLin below.
	gain := g.rangeLin
	if g.envelope >= g.thresholdLin {
		gain = 1.0
	}

	return x * gain
}

func (g *gate) Reset() {
	g.envelope = 0
}

func (g *gate) SetParam(name string, value float64) {
	switch name {
	case "threshold":
		g.thresholdDB = clampf(value, -60, 0)
		g.recalc()
	case "attack":
		g.attackMs = clampf(value, 0.1, 50)
		g.recalc()
	case "release":
		g.releaseMs = clampf(value, 1, 500)
		g.recalc()
	case "range":
		g.rangeDB = clampf(value, -90, 0)
		g.recalc()
	}
}
