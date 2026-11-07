package wavref

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

// go test runs from the package dir and go run from src/go, so cmd/refs needs an absolute anchor.
func ModuleRoot() (string, bool) {
	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// Inside Root() only, with no in-tree fallback. ok=false when unknown, a status entry, or absent: callers skip.
func Resolve(id string) (string, bool) {
	c, err := Catalogue()
	if err != nil {
		return "", false
	}
	r, ok := c.Ref(id)
	if !ok || r.File == "" {
		return "", false
	}
	p := filepath.Join(Root(), r.File)
	if _, err := os.Stat(p); err != nil {
		return "", false
	}
	return p, true
}

// Rate, count, then every sample as big-endian int16: the same sound, whatever metadata an ffmpeg build wrote. Callers
// decode first: importing internal/audio would close an import cycle through audio's tests.
func PCMHash(samples []float32, rate int) string {
	h := sha256.New()
	var scratch [8]byte
	binary.BigEndian.PutUint32(scratch[:4], uint32(rate))
	binary.BigEndian.PutUint32(scratch[4:8], uint32(len(samples)))
	h.Write(scratch[:])
	var s2 [2]byte
	for _, f := range samples {
		v := int16(math.Round(math.Max(-32768, math.Min(32767, float64(f)*32767))))
		binary.BigEndian.PutUint16(s2[:], uint16(v))
		h.Write(s2[:])
	}
	return hex.EncodeToString(h.Sum(nil))
}

// No recorded hash verifies vacuously: absence is a fetch concern, not corruption. The caller resolves, decodes and
// hashes (the import cycle again).
func VerifyHash(id, gotHash string) error {
	c, err := Catalogue()
	if err != nil {
		return err
	}
	r, ok := c.Ref(id)
	if !ok {
		return fmt.Errorf("wavref: unknown ref %q", id)
	}
	return verifyRefHash(r, gotHash)
}

// Split out so the mismatch path is testable without a catalogue entry carrying a pcm_sha256.
func verifyRefHash(r Ref, gotHash string) error {
	if r.PCMSHA256 == "" {
		return nil
	}
	if gotHash != r.PCMSHA256 {
		return fmt.Errorf("wavref: %s: pcm hash mismatch\n  want %s\n  got  %s", r.ID, r.PCMSHA256, gotHash)
	}
	return nil
}
