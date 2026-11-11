//go:build test

package audio

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// TestDecodeWAVToPCM_24bit decodes a GENERATED 24-bit 48 kHz WAV. It used to
// read the freesound violin original by filename from the reference root, but
// the catalogue stores that id's `refs fetch` conversion (16-bit 44.1 kHz)
// under the same name, so the test was asserting a format the catalogued file
// does not have. Reference audio is never committed; test audio is generated.
func TestDecodeWAVToPCM_24bit(t *testing.T) {
	const sr, n = 48000, 125000 // ~2.6 s
	data := make([]byte, 0, 44+3*n)
	le32 := func(v uint32) { data = binary.LittleEndian.AppendUint32(data, v) }
	le16 := func(v uint16) { data = binary.LittleEndian.AppendUint16(data, v) }
	data = append(data, "RIFF"...)
	le32(uint32(36 + 3*n))
	data = append(data, "WAVEfmt "...)
	le32(16)
	le16(1)      // PCM
	le16(1)      // mono
	le32(sr)     // sample rate
	le32(sr * 3) // byte rate
	le16(3)      // block align
	le16(24)     // bits per sample
	data = append(data, "data"...)
	le32(uint32(3 * n))
	for i := 0; i < n; i++ {
		v := int32(math.Round(0.5 * math.Sin(2*math.Pi*587.33*float64(i)/sr) * 8388607))
		data = append(data, byte(v), byte(v>>8), byte(v>>16))
	}
	path := filepath.Join(t.TempDir(), "sine24.wav")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	pcm, gotSR, err := DecodeWAVToPCM(path)
	if err != nil {
		t.Fatalf("decode 24-bit: %v", err)
	}
	if gotSR != sr {
		t.Fatalf("sample rate = %d, want %d", gotSR, sr)
	}
	if len(pcm) != n {
		t.Fatalf("len(pcm) = %d, want %d", len(pcm), n)
	}
	var peak float32
	for _, s := range pcm {
		if s > peak {
			peak = s
		} else if -s > peak {
			peak = -s
		}
	}
	if math.Abs(float64(peak)-0.5) > 0.01 {
		t.Fatalf("peak = %.4f, want 0.5 (24-bit samples mis-scaled)", peak)
	}
}
