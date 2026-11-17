package audio

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math"
)

// content_hash.go carries NO build tags on purpose: the WAV version pin must
// hash identically on native, wasm, and under -tags test. crypto/sha256 is
// pure-Go stdlib and compiles for js/wasm.

// Sha256Hex returns the lowercase hex SHA-256 of b.
func Sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// HashSamplePCM returns a stable content hash of a decoded sample: every frame
// as little-endian float32 followed by the sample rate. This is the pinned
// "version" a WavRef records — sharing the instrument config carries only this
// hash, never the audio bytes.
func HashSamplePCM(pcm []float32, sampleRate int) string {
	buf := make([]byte, 0, len(pcm)*4+4)
	var w [4]byte
	for _, s := range pcm {
		binary.LittleEndian.PutUint32(w[:], math.Float32bits(s))
		buf = append(buf, w[:]...)
	}
	binary.LittleEndian.PutUint32(w[:], uint32(sampleRate))
	buf = append(buf, w[:]...)
	return Sha256Hex(buf)
}
