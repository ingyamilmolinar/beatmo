package audio

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math"
)

// Untagged: the WAV version pin must hash identically on native, wasm and under -tags test.

// Sha256Hex returns the lowercase hex SHA-256 of b.
func Sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Every frame as little-endian float32, then the rate: the version a WavRef pins, so sharing carries the hash, never
// the audio.
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
