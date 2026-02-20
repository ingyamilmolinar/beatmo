//go:build test || js

package audio

import "math"

// bitcrusher reduces bit depth and sample rate for lo-fi effects.
// "bits" controls quantization (2-16 bit), "rate" controls decimation
// (1.0 = no reduction, 0.01 = extreme reduction), and "mix" blends wet/dry.
type bitcrusher struct {
	bits float64 // 2-16
	rate float64 // 0.01-1.0 (fraction of original sample rate)
	mix  float64 // wet/dry

	// Sample-hold state
	holdCounter float64 // fractional counter for decimation
	holdValue   float64 // last held sample
}

func newBitcrusher(sr int, params map[string]float64) *bitcrusher {
	b := &bitcrusher{}
	b.bits = clampf(params["bits"], 2, 16)
	b.rate = clampf(params["rate"], 0.01, 1)
	b.mix = clampf(params["mix"], 0, 1)
	return b
}

func (b *bitcrusher) ProcessSample(x float64) float64 {
	// Sample rate reduction via sample-and-hold
	b.holdCounter += b.rate
	if b.holdCounter >= 1.0 {
		b.holdCounter -= 1.0
		// Bit depth reduction: quantize to fewer levels
		levels := math.Pow(2, b.bits)
		// Scale to [0, levels], round, scale back to [-1, 1]
		b.holdValue = math.Round(x*levels) / levels
	}
	wet := b.holdValue
	return x*(1-b.mix) + wet*b.mix
}

func (b *bitcrusher) Reset() {
	b.holdCounter = 0
	b.holdValue = 0
}

func (b *bitcrusher) SetParam(name string, value float64) {
	switch name {
	case "bits":
		b.bits = clampf(value, 2, 16)
	case "rate":
		b.rate = clampf(value, 0.01, 1)
	case "mix":
		b.mix = clampf(value, 0, 1)
	}
}
