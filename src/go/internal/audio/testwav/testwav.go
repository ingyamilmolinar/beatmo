// Package testwav writes minimal, valid PCM WAV files into a t.TempDir() and
// returns audio.SoundMeta entries that point at them. Test code uses these to
// drive the catalog without depending on any embedded or repo-tracked sample
// bytes.
package testwav

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

const sampleRate = 44100

// Spec describes one synthetic WAV to stage.
type Spec struct {
	ID       string
	Name     string
	Category string
	// Filename, optional. Defaults to ID + ".wav".
	Filename string
}

// Stage writes one tiny WAV per spec into a fresh subdirectory of t.TempDir()
// and returns SoundMeta entries with absolute Path values. The catalog is not
// touched; callers may pass the result to audio.ResetCatalogForTest.
func Stage(t testing.TB, specs []Spec) []audio.SoundMeta {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "wav")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("testwav: mkdir %s: %v", dir, err)
	}
	out := make([]audio.SoundMeta, 0, len(specs))
	for _, s := range specs {
		name := s.Filename
		if name == "" {
			name = s.ID + ".wav"
		}
		path := filepath.Join(dir, name)
		if err := writeTinyWAV(path); err != nil {
			t.Fatalf("testwav: write %s: %v", path, err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("testwav: stat %s: %v", path, err)
		}
		out = append(out, audio.SoundMeta{
			ID:         s.ID,
			Name:       s.Name,
			Category:   s.Category,
			RelPath:    name,
			Path:       filepath.ToSlash(path),
			Size:       info.Size(),
			Source:     "wav",
			Scope:      "shipped",
			SampleRate: sampleRate,
			Channels:   1,
		})
	}
	return out
}

// WritePath writes a single tiny WAV at an explicit path. Used when tests need
// the file at a specific location (e.g. mirroring a project layout).
func WritePath(t testing.TB, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("testwav: mkdir parent of %s: %v", path, err)
	}
	if err := writeTinyWAV(path); err != nil {
		t.Fatalf("testwav: write %s: %v", path, err)
	}
}

func writeTinyWAV(path string) error {
	const samples = sampleRate / 200 // 5ms
	data := make([]int16, samples)
	for i := range data {
		data[i] = int16(math.Sin(2*math.Pi*float64(i)/float64(samples)) * 12000)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	dataSize := uint32(len(data) * 2)
	if _, err := f.Write([]byte("RIFF")); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, 36+dataSize); err != nil {
		return err
	}
	if _, err := f.Write([]byte("WAVEfmt ")); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(16)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint16(1)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint16(1)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(sampleRate)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(sampleRate*2)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint16(2)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint16(16)); err != nil {
		return err
	}
	if _, err := f.Write([]byte("data")); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, dataSize); err != nil {
		return err
	}
	for _, v := range data {
		if err := binary.Write(f, binary.LittleEndian, v); err != nil {
			return err
		}
	}
	return nil
}
