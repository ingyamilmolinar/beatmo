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

// ModuleRoot walks up from the working directory to the directory holding
// go.mod. Callers run from different cwds (go test uses the package dir,
// go run uses src/go), so an absolute anchor is required. Exported because
// cmd/refs needs it to locate the templates dir and catalogue.json without
// counting filepath.Dir calls.
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

// Resolve locates the audio file for a catalogue id inside the one
// reference-audio root (Root()). There is no in-tree fallback: reference
// audio is never committed. ok=false when the id is unknown, carries a status
// instead of audio, or the file is absent — callers skip rather than fail.
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

// PCMHash hashes DECODED audio — sample rate, sample count, then every
// sample as a big-endian int16 — rather than container bytes. ffmpeg builds
// differ in the metadata chunks they emit; the audio does not, so hashing
// PCM is what makes verification mean "same sound".
//
// This package deliberately does NOT import internal/audio to decode the
// file itself: internal/audio's own test files (wavref_ref_test.go) import
// internal/wavref, so wavref -> audio would close an import cycle
// (audio [test] -> wavref -> audio). Callers decode first (e.g. via
// audio.DecodeWAVToPCM) and pass the resulting samples/rate in here.
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

// VerifyHash compares a caller-computed PCM hash against the catalogue's
// recorded value for id. A ref with no recorded hash verifies vacuously —
// absence is a fetch concern, not a corruption concern. The caller is
// responsible for resolving and decoding the audio (and for PCMHash) before
// calling this, for the same import-cycle reason PCMHash takes samples
// rather than a path.
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

// verifyRefHash is the pure comparison VerifyHash delegates to, split out so
// the mismatch path is unit-testable without depending on catalogue.json
// carrying a populated pcm_sha256 (that population is Task 5's job; today's
// 5 refs all ship without one).
func verifyRefHash(r Ref, gotHash string) error {
	if r.PCMSHA256 == "" {
		return nil
	}
	if gotHash != r.PCMSHA256 {
		return fmt.Errorf("wavref: %s: pcm hash mismatch\n  want %s\n  got  %s", r.ID, r.PCMSHA256, gotHash)
	}
	return nil
}
