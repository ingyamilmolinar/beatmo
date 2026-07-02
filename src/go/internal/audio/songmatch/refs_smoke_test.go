package songmatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

func TestSongRefs_DecodeAndFingerprint(t *testing.T) {
	dir := filepath.Join("..", "testdata", "songrefs")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skip("no songrefs dir; run scripts/fetch_song_refs.sh")
	}
	var wavs []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".wav") {
			wavs = append(wavs, e.Name())
		}
	}
	if len(wavs) == 0 {
		t.Skip("no reference WAVs present; run scripts/fetch_song_refs.sh")
	}
	audio.Reset()
	audio.ResetInstruments()
	cfg := fingerprint.DefaultAnalysisConfig()
	for _, name := range wavs {
		pcm, sr, err := audio.DecodeWAVToPCM(filepath.Join(dir, name))
		if err != nil {
			t.Errorf("%s: decode: %v", name, err)
			continue
		}
		f := make([]float64, len(pcm))
		for i, v := range pcm {
			f[i] = float64(v)
		}
		fp := fingerprint.SongFingerprintOf(wave.Wave{Samples: f, SampleRate: sr}, cfg)
		if fp.TempoBPM <= 0 || fp.TempoBPM > 400 {
			t.Errorf("%s: implausible tempo %.1f BPM", name, fp.TempoBPM)
		}
	}
}
