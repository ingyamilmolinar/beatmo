//go:build test

package ui

// Byte-exact: determinism (no hidden state), shape (the analyser covers every display band at every shipped
// rate) and a golden the browser must reproduce from the same Go code and input.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audiotap"
)

// tapGoldenPath is shared with the browser test, which reads the same file.
const tapGoldenPath = "testdata/audio_tab_tap_golden.json"

// Through the real render derivations.
func captureCanonical(t *testing.T) *audiotap.Snapshot {
	t.Helper()
	const sr = 48000
	st := CanonicalAnalyzerState(audio.BridgeSpectrumBins, audio.BridgeSpectrumBins, sr)
	return CaptureAudioTabTap("canonical/dnb-kick", st.Detail, st, sr)
}

func TestAudioTabTapIsDeterministic(t *testing.T) {
	a := captureCanonical(t)
	b := captureCanonical(t)
	if diffs := a.Diff(b); len(diffs) > 0 {
		t.Fatalf("the same input captured twice produced different bytes — something in the "+
			"derivation carries state across calls:\n%s", audiotap.Report(diffs))
	}
	if a.Overall() != b.Overall() {
		t.Fatalf("overall digest differs across identical captures: %s != %s", a.Overall(), b.Overall())
	}
}

// Otherwise the golden could pin a digest blind to the very defect it exists for.
func TestAudioTabTapDetectsAResolutionChange(t *testing.T) {
	const sr = 48000
	good := CanonicalAnalyzerState(audio.BridgeSpectrumBins, audio.BridgeSpectrumBins, sr)
	// The shipped-broken shape: 64 linear bins over 0..Nyquist.
	bad := CanonicalAnalyzerState(64, 64, sr)

	gs := CaptureAudioTabTap("x", good.Detail, good, sr)
	bs := CaptureAudioTabTap("x", bad.Detail, bad, sr)

	diffs := gs.Diff(bs)
	if len(diffs) == 0 {
		t.Fatal("a 64-bin spectrum digested identically to a full-resolution one — " +
			"the capture is blind to the defect it exists to catch")
	}
	// It must specifically name the SHAPE layer, not just "something moved".
	var sawShape, sawBandCounts bool
	for _, d := range diffs {
		if len(d) > 6 && d[:6] == "shape." {
			sawShape = true
		}
		if len(d) > 22 && d[:22] == "shape.band_bin_counts:" {
			sawBandCounts = true
		}
	}
	if !sawShape {
		t.Errorf("resolution change did not move any shape.* section; diffs:\n%s", audiotap.Report(diffs))
	}
	if !sawBandCounts {
		t.Errorf("resolution change did not move shape.band_bin_counts — that section is the "+
			"direct read of 'does every display band contain a bin';\n%s", audiotap.Report(diffs))
	}
}

// Stated over the capture, not a hand-built spectrum.
func TestAudioTabTapBandCoverageAtEveryShippedRate(t *testing.T) {
	for _, sr := range []int{44100, 48000} {
		st := CanonicalAnalyzerState(audio.BridgeSpectrumBins, audio.BridgeSpectrumBins, sr)
		snap := CaptureAudioTabTap("coverage", st.Detail, st, sr)
		for _, sec := range snap.Sections {
			if sec.Name != "render.spectrum.band_measured" {
				continue
			}
			_, measured := spectrumBandDB(st.Detail.FFTBins, st.Detail.FreqBins, isoBands)
			for b, ok := range measured {
				if !ok {
					t.Errorf("sr=%d: display band %d (%.0f-%.0f Hz) contains no FFT bin — it will "+
						"report the reference floor whatever the signal does",
						sr, b, isoBands[b][0], isoBands[b][1])
				}
			}
		}
	}
}

func TestAudioTabTapMatchesGolden(t *testing.T) {
	got := captureCanonical(t)

	if os.Getenv("AUDIO_TAP_GOLDEN_UPDATE") == "1" {
		b, err := json.MarshalIndent(got, "", "  ")
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if err := os.WriteFile(tapGoldenPath, append(b, '\n'), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("golden updated: %s (overall %s)", tapGoldenPath, got.Overall())
		return
	}

	raw, err := os.ReadFile(tapGoldenPath)
	if err != nil {
		t.Fatalf("read golden (regenerate with AUDIO_TAP_GOLDEN_UPDATE=1): %v", err)
	}
	var want audiotap.Snapshot
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatalf("parse golden: %v", err)
	}
	if diffs := got.Diff(&want); len(diffs) > 0 {
		t.Fatalf("audio-tab capture drifted from the committed golden.\n%s\n"+
			"If the change is intended, regenerate with:\n"+
			"  AUDIO_TAP_GOLDEN_UPDATE=1 go test -tags test -run TestAudioTabTapMatchesGolden ./internal/ui/\n"+
			"and re-run the browser half (xplat_audio_tab_tap.browser.test.js), which "+
			"compares against this same file.", audiotap.Report(diffs))
	}
}

// The browser test resolves the golden by this relative path.
func TestAudioTabTapGoldenPathIsReadableFromJS(t *testing.T) {
	abs, err := filepath.Abs(tapGoldenPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(abs); err != nil {
		t.Fatalf("golden missing at %s: %v", abs, err)
	}
}
