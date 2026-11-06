//go:build !test && !js

package audio

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Shared helpers for the native (CGo) render tests: one fixed render shape and
// a byte-exact digest, so every byte-identity test hashes the same way.

const (
	nativeGoldenSR      = 48000
	nativeGoldenSamples = 48000 // 1.0s
)

// hashFloat32 is the sha256 over a buffer's raw IEEE-754 bits — any one-ULP
// change in any sample changes it.
func hashFloat32(buf []float32) string {
	h := sha256.New()
	var b [4]byte
	for _, v := range buf {
		binary.LittleEndian.PutUint32(b[:], math.Float32bits(v))
		h.Write(b[:])
	}
	return hex.EncodeToString(h.Sum(nil))
}

// pickNonDefaultValue returns a value of d as far from its default as its range
// allows (the opposite end), so an edit to it is maximally likely to be heard.
func pickNonDefaultValue(d ParamDef) float64 {
	if d.Min == d.Max {
		return d.Default
	}
	mid := (d.Min + d.Max) / 2
	if d.Default >= mid {
		return d.Min
	}
	return d.Max
}

// shippedRender returns a Render func playing instrument id exactly as shipped:
// its document's flattened block, RAW.
func shippedRender(id instrument.ID) func(buf []float32, sampleRate, samples int) {
	return func(buf []float32, sampleRate, samples int) {
		if out, err := RenderInstrumentWithFields(id, sampleRate, samples, nil); err == nil {
			copy(buf, out)
		}
	}
}

// shippedSnareVoice is a real shipped snare voice for the mixer smoke tests.
func shippedSnareVoice(sampleRate int) Voice {
	return docInstrument{id: roster.Snare}.NewVoice(sampleRate)
}
