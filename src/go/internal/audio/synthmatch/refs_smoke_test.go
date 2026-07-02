package synthmatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

func wavsIn(dir string) []string {
	var out []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".wav") {
			out = append(out, e.Name())
		}
	}
	return out
}

// TestRefs_DecodeAndFingerprint verifies every committed reference WAV decodes
// and fingerprints with a plausible F0. Skips cleanly when refs/ is empty, so
// the suite is green before scripts/fetch_synth_refs.sh is ever run.
func TestRefs_DecodeAndFingerprint(t *testing.T) {
	dir := filepath.Join("..", "testdata", "refs") // internal/audio/testdata/refs
	names := wavsIn(dir)
	if len(names) == 0 {
		t.Skip("no reference WAVs present; run scripts/fetch_synth_refs.sh")
	}
	audio.Reset()
	audio.ResetInstruments()
	for _, name := range names {
		pcm, sr, err := audio.DecodeWAVToPCM(filepath.Join(dir, name))
		if err != nil {
			t.Errorf("%s: decode: %v", name, err)
			continue
		}
		f64 := make([]float64, len(pcm))
		for i, v := range pcm {
			f64[i] = float64(v)
		}
		fp := fingerprint.FromWave(wave.Wave{Samples: f64, SampleRate: sr}, name)
		if fp.F0Hz <= 0 || fp.F0Hz > 8000 {
			t.Errorf("%s: implausible F0=%.1f", name, fp.F0Hz)
		}
	}
}
