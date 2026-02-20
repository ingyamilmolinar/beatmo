//go:build test || js

package audio

import "math"

// bitcrusher reduces bit depth and sample rate for lo-fi effects.
// "bits" controls quantization (2-16 bit), "rate" controls decimation
// (1.0 = no reduction, 0.01 = extreme reduction), and "mix" blends wet/dry.
// All parameters are smoothed to prevent clicks on change.
type bitcrusher struct {
	bits smoothParam // 2-16
	rate smoothParam // 0.01-1.0 (fraction of original sample rate)
	mix  smoothParam // wet/dry

	// Sample-hold state
	holdCounter float64 // fractional counter for decimation
	holdValue   float64 // last held sample
}

func newBitcrusher(sr int, params map[string]float64) *bitcrusher {
	b := &bitcrusher{}
	b.bits = newSmoothParam(clampf(params["bits"], 2, 16), sr, defaultSmoothTimeMs)
	b.rate = newSmoothParam(clampf(params["rate"], 0.01, 1), sr, defaultSmoothTimeMs)
	b.mix = newSmoothParam(clampf(params["mix"], 0, 1), sr, defaultSmoothTimeMs)
	return b
}

func (b *bitcrusher) ProcessSample(x float64) float64 {
	bits := b.bits.tick()
	rate := b.rate.tick()
	mix := b.mix.tick()

	// Sample rate reduction via sample-and-hold
	b.holdCounter += rate
	if b.holdCounter >= 1.0 {
		b.holdCounter -= 1.0
		// Bit depth reduction: quantize to fewer levels
		levels := math.Pow(2, bits)
		// Scale to [0, levels], round, scale back to [-1, 1]
		b.holdValue = math.Round(x*levels) / levels
	}
	wet := b.holdValue
	return x*(1-mix) + wet*mix
}

func (b *bitcrusher) Reset() {
	b.holdCounter = 0
	b.holdValue = 0
}

func (b *bitcrusher) SetParam(name string, value float64) {
	switch name {
	case "bits":
		b.bits.set(clampf(value, 2, 16))
	case "rate":
		b.rate.set(clampf(value, 0.01, 1))
	case "mix":
		b.mix.set(clampf(value, 0, 1))
	}
}
