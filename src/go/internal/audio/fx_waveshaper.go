//go:build test || js

package audio

import "math"

// waveshaper implements a nonlinear waveshaping effect with four curve types:
// soft (tanh), hard (clip), fold (wave folding), and sine. Drive controls
// the amount of gain applied before the shaping function.
type waveshaper struct {
	curve float64 // 0=soft/tanh, 1=hard/clip, 2=fold, 3=sine
	drive float64 // pre-shape gain 1-20
	mix   float64 // wet/dry 0-1
}

func newWaveshaper(sr int, params map[string]float64) *waveshaper {
	w := &waveshaper{}
	w.curve = clampf(params["curve"], 0, 3)
	w.drive = clampf(params["drive"], 1, 20)
	w.mix = clampf(params["mix"], 0, 1)
	return w
}

func (w *waveshaper) ProcessSample(x float64) float64 {
	driven := x * w.drive

	var wet float64
	curveType := int(w.curve + 0.5) // round to nearest int
	switch curveType {
	case 0: // soft clip (tanh)
		wet = math.Tanh(driven)
	case 1: // hard clip
		wet = clampf(driven, -1, 1)
	case 2: // wave folding
		wet = driven
		for i := 0; i < 10; i++ {
			if wet >= -1.0 && wet <= 1.0 {
				break
			}
			if wet > 1.0 {
				wet = 2.0 - wet
			}
			if wet < -1.0 {
				wet = -2.0 - wet
			}
		}
		// Final clamp in case loop didn't converge
		wet = clampf(wet, -1, 1)
	case 3: // sine shaper
		wet = math.Sin(driven)
	default:
		wet = math.Tanh(driven)
	}

	return x*(1-w.mix) + wet*w.mix
}

func (w *waveshaper) Reset() {
	// Waveshaper is stateless — nothing to reset.
}

func (w *waveshaper) SetParam(name string, value float64) {
	switch name {
	case "curve":
		w.curve = clampf(value, 0, 3)
	case "drive":
		w.drive = clampf(value, 1, 20)
	case "mix":
		w.mix = clampf(value, 0, 1)
	}
}
