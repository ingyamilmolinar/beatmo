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

// One render shape and one digest, so every byte-identity test hashes the same way.

const (
	nativeGoldenSR      = 48000
	nativeGoldenSamples = 48000 // 1.0s
)

// Any one-ULP change in any sample changes it.
func hashFloat32(buf []float32) string {
	h := sha256.New()
	var b [4]byte
	for _, v := range buf {
		binary.LittleEndian.PutUint32(b[:], math.Float32bits(v))
		h.Write(b[:])
	}
	return hex.EncodeToString(h.Sum(nil))
}

// The opposite end of the range, so an edit is most likely to be heard.
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

// The document's flattened block, RAW.
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
