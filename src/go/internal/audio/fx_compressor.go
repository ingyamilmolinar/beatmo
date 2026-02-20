//go:build test || js

package audio

import "math"

// compressorFX implements a per-instrument dynamics compressor as an insert
// effect. It uses a peak envelope follower with separate attack/release
// coefficients, hard-knee gain computation, optional makeup gain, and
// wet/dry mix. All user-facing parameters are smoothed via smoothParam to
// prevent clicks on change. The envelope follower itself is NOT smoothed —
// it tracks signal amplitude in real time.
//
// This is distinct from the master-bus Compressor type.
type compressorFX struct {
	threshold smoothParam // -60..0 dB
	ratio     smoothParam // 1..20
	attack    smoothParam // 0.1..100 ms (stored as ms, coefs recomputed per sample)
	release   smoothParam // 10..1000 ms
	makeup    smoothParam // 0..24 dB
	mix       smoothParam // 0..1

	sr       int
	envelope float64 // peak envelope follower state (linear amplitude)
}

func newCompressorFX(sr int, params map[string]float64) *compressorFX {
	c := &compressorFX{sr: sr}
	c.threshold = newSmoothParam(clampf(params["threshold"], -60, 0), sr, defaultSmoothTimeMs)
	c.ratio = newSmoothParam(clampf(params["ratio"], 1, 20), sr, defaultSmoothTimeMs)
	c.attack = newSmoothParam(clampf(params["attack"], 0.1, 100), sr, defaultSmoothTimeMs)
	c.release = newSmoothParam(clampf(params["release"], 10, 1000), sr, defaultSmoothTimeMs)
	c.makeup = newSmoothParam(clampf(params["makeup"], 0, 24), sr, defaultSmoothTimeMs)
	c.mix = newSmoothParam(clampf(params["mix"], 0, 1), sr, defaultSmoothTimeMs)
	return c
}

func (c *compressorFX) ProcessSample(x float64) float64 {
	threshDB := c.threshold.tick()
	rat := c.ratio.tick()
	attackMs := c.attack.tick()
	releaseMs := c.release.tick()
	makeupDB := c.makeup.tick()
	mix := c.mix.tick()

	// Compute envelope coefficients from current (smoothed) attack/release times.
	sr := float64(c.sr)
	if sr <= 0 {
		sr = 44100
	}
	attackCoef := math.Exp(-1.0 / (attackMs * 0.001 * sr))
	releaseCoef := math.Exp(-1.0 / (releaseMs * 0.001 * sr))

	// Peak envelope follower.
	absX := math.Abs(x)
	if absX > c.envelope {
		c.envelope = absX + (c.envelope-absX)*attackCoef
	} else {
		c.envelope = absX + (c.envelope-absX)*releaseCoef
	}

	// Convert envelope to dB.
	envDB := 20.0 * math.Log10(math.Max(c.envelope, 1e-10))

	// Hard-knee gain computation.
	var gainDB float64
	if envDB > threshDB {
		gainDB = threshDB + (envDB-threshDB)/rat - envDB
	}

	// Add makeup gain.
	gainDB += makeupDB

	// Convert to linear gain and apply.
	gain := math.Pow(10.0, gainDB/20.0)
	wet := x * gain

	// Wet/dry mix.
	return x*(1-mix) + wet*mix
}

func (c *compressorFX) Reset() {
	c.envelope = 0
}

func (c *compressorFX) SetParam(name string, value float64) {
	switch name {
	case "threshold":
		c.threshold.set(clampf(value, -60, 0))
	case "ratio":
		c.ratio.set(clampf(value, 1, 20))
	case "attack":
		c.attack.set(clampf(value, 0.1, 100))
	case "release":
		c.release.set(clampf(value, 10, 1000))
	case "makeup":
		c.makeup.set(clampf(value, 0, 24))
	case "mix":
		c.mix.set(clampf(value, 0, 1))
	}
}
