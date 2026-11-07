//go:build !test && !js

package audio

import (
	"fmt"
	"math"
	"slices"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// (sampleCount, order-sensitive checksum of every sample's float32 bits).
func synthRenderChecksum(id string) (int, uint64) {
	buf, _ := RenderInstrumentOneShotRaw(instrument.ID(id))
	var bits uint64
	for i, v := range buf {
		bits ^= (uint64(i)*2654435761 + 1) * uint64(math.Float32bits(v))
	}
	return len(buf), bits
}

// Regenerates the pinned goldens: go test -run TestSynthGoldenIdentityCapture -v ./internal/audio/ | grep GOLDEN
func TestSynthGoldenIdentityCapture(t *testing.T) {
	Reset()
	ResetInstruments()
	ids := append([]instrument.ID(nil), roster.All...)
	slices.Sort(ids)
	for _, id := range ids {
		n, bits := synthRenderChecksum(string(id))
		fmt.Printf("GOLDEN %q: {%d, 0x%016x},\n", id, n, bits)
	}
}
