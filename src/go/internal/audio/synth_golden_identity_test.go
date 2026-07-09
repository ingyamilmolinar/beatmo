//go:build !test && !js

package audio

import (
	"fmt"
	"math"
	"sort"
	"testing"
)

// synthRenderChecksum renders an instrument one-shot deterministically and
// returns (sampleCount, bitChecksum). The bit checksum is order-sensitive and
// covers every sample's exact float32 bits — a single changed sample changes it.
func synthRenderChecksum(id string) (int, uint64) {
	buf, _ := RenderInstrumentOneShotRaw(id)
	var bits uint64
	for i, v := range buf {
		bits ^= (uint64(i)*2654435761 + 1) * uint64(math.Float32bits(v))
	}
	return len(buf), bits
}

// TestSynthGoldenIdentityCapture prints the render checksum for every built-in
// instrument. Run with CAPTURE to (re)generate the pinned goldens in
// TestSynthGoldenByteIdentity:
//
//	go test -run TestSynthGoldenIdentityCapture -v ./internal/audio/ 2>&1 | grep GOLDEN
func TestSynthGoldenIdentityCapture(t *testing.T) {
	Reset()
	ResetInstruments()
	ids := append([]string(nil), BuiltinInstrumentIDs...)
	sort.Strings(ids)
	for _, id := range ids {
		n, bits := synthRenderChecksum(id)
		fmt.Printf("GOLDEN %q: {%d, 0x%016x},\n", id, n, bits)
	}
}
