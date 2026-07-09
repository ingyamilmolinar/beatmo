package audio

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
)

// parseJSAmpMap parses `src/js/instrument_loudness.gen.js` entries of the form
//
//	"id": 0.1234,
func parseJSAmpMap(t *testing.T) map[string]float32 {
	t.Helper()
	// test cwd is the package dir: src/go/internal/audio → repo/src/js.
	// (src/go/internal/audio) ../../../ == repo/src, so no extra "src" segment.
	path := filepath.Join("..", "..", "..", "js", "instrument_loudness.gen.js")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated JS: %v (run `make measure-loudness`)", err)
	}
	re := regexp.MustCompile(`"([^"]+)":\s*([0-9.]+)`)
	out := map[string]float32{}
	for _, m := range re.FindAllStringSubmatch(string(data), -1) {
		f, err := strconv.ParseFloat(m[2], 32)
		if err != nil {
			t.Fatalf("parse %q: %v", m[2], err)
		}
		out[m[1]] = float32(f)
	}
	return out
}

func TestLoudnessAmpGoJSParity(t *testing.T) {
	js := parseJSAmpMap(t)
	if len(js) != len(instrumentLoudnessAmp) {
		t.Fatalf("entry count mismatch: go=%d js=%d — regenerate with `make measure-loudness`",
			len(instrumentLoudnessAmp), len(js))
	}
	for id, goAmp := range instrumentLoudnessAmp {
		if jsAmp, ok := js[id]; !ok || jsAmp != goAmp {
			t.Errorf("amp mismatch for %q: go=%v js=%v (ok=%v)", id, goAmp, jsAmp, ok)
		}
	}
}

func TestLoudnessAmpCoversAllInstruments(t *testing.T) {
	for id := range InstrumentConfigs {
		if _, ok := instrumentLoudnessAmp[id]; !ok {
			t.Errorf("instrument %q has no loudness amp — run `make measure-loudness`", id)
		}
	}
}
