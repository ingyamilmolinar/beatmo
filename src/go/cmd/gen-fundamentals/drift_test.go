package main

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/audio/synthmatch"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// generatedPath is the committed table this guard defends.
const generatedPath = "../../internal/audio/instrument_fundamentals.gen.go"

// A stale table is a wrong note name, silently, on both platforms, and the browser cannot re-measure (Go on wasm never
// holds the C render's PCM). It compares the rendered SOURCE, so a generator output change is caught too.
func TestInstrumentFundamentalsUpToDate(t *testing.T) {
	if renderIsStub {
		// A visible skip, not a build tag: under -tags test the render is a stub, and a tagged-out file would report a silent pass.
		t.Skip("render path is the -tags test stub; run without -tags test (needs CGO + build/libdrums.a)")
	}

	want, err := os.ReadFile(generatedPath)
	if err != nil {
		t.Fatalf("reading committed table: %v", err)
	}

	ms, err := MeasureAll()
	if err != nil {
		t.Fatalf("re-measuring the roster: %v", err)
	}
	got, err := Render(ms)
	if err != nil {
		t.Fatalf("rendering table source: %v", err)
	}

	if string(got) != string(want) {
		abs, _ := filepath.Abs(generatedPath)
		t.Errorf("%s is stale: an instrument's measured fundamental has changed.\n"+
			"Run `make gen-fundamentals` and review the diff — a moved fundamental means a\n"+
			"node's displayed note name has changed, so confirm the retune was intended.", abs)
	}
}

// The band agreementToleranceSemis sits in must stay wide and empty: it separates a pitch-envelope drop below the
// oscillator setting from an octave-class detector error.
func TestAgreementBandIsCalibrated(t *testing.T) {
	if renderIsStub {
		t.Skip("render path is the -tags test stub; run without -tags test (needs CGO + build/libdrums.a)")
	}
	ms, err := MeasureAll()
	if err != nil {
		t.Fatalf("re-measuring the roster: %v", err)
	}

	worst, worstID := 0.0, ""
	for _, m := range ms {
		if m.Source != SourceMeasured || m.DeclaredHz <= 0 {
			continue
		}
		if d := semitonesBetween(m.MeasuredHz, m.DeclaredHz); d > worst {
			worst, worstID = d, string(m.ID)
		}
	}

	const octaveClassSemis = 12.0 // the smallest error this guard must catch
	const minMargin = 2.0
	if worst > agreementToleranceSemis-minMargin {
		t.Errorf("largest legitimate disagreement is %s at %.2f semitones, within %.2f of the "+
			"%.2f tolerance — the band is too tight to separate a pitch-envelope drop from a detector error",
			worstID, worst, minMargin, agreementToleranceSemis)
	}
	if octaveClassSemis-agreementToleranceSemis < minMargin {
		t.Errorf("tolerance %.2f leaves only %.2f semitones of margin below an octave-class error (%.2f); "+
			"it would stop catching the errors it exists for",
			agreementToleranceSemis, octaveClassSemis-agreementToleranceSemis, octaveClassSemis)
	}
	t.Logf("band: largest legitimate disagreement %.2f (%s) < tolerance %.2f < octave-class %.2f",
		worst, worstID, agreementToleranceSemis, octaveClassSemis)
}

// The lowest significant partial of a real pitch sits within its first few harmonics (~3x for a missing fundamental);
// a cymbal's sits at 6x or beyond. One threshold separates them only while that gap stays wide and empty.
func TestF0SupportBandIsCalibrated(t *testing.T) {
	if renderIsStub {
		t.Skip("render path is the -tags test stub; run without -tags test (needs CGO + build/libdrums.a)")
	}

	// The threshold must sit between these with real margin on each side.
	supportedMax, supportedMaxID := 0.0, ""
	rejectedMin, rejectedMinID := math.Inf(1), ""

	for _, id := range roster.All {
		w, err := synthmatch.RenderInstrument(id, 0, renderSR, renderSec)
		if err != nil {
			t.Fatalf("render %s: %v", id, err)
		}
		est := fingerprint.DetectF0Detailed(w)
		sup := fingerprint.AnalyzeF0Support(w, est.Hz)
		if sup.LowestPartialRatio <= 0 {
			continue // no partial found at all; the ratio axis says nothing here
		}
		if sup.Supported {
			if sup.LowestPartialRatio > supportedMax {
				supportedMax, supportedMaxID = sup.LowestPartialRatio, string(id)
			}
			continue
		}
		if sup.LowestPartialRatio < rejectedMin {
			rejectedMin, rejectedMinID = sup.LowestPartialRatio, string(id)
		}
	}

	if supportedMax <= 0 || math.IsInf(rejectedMin, 1) {
		t.Fatalf("one side of the band is empty (supportedMax=%.2f rejectedMin=%.2f); "+
			"the calibration cannot be checked", supportedMax, rejectedMin)
	}

	// Express the margins in semitones: the ratio axis is multiplicative, so a
	// fixed additive margin would mean something different at each end.
	const minMarginSemis = 3.0
	gapSemis := 12 * math.Log2(rejectedMin/supportedMax)
	if gapSemis < 2*minMarginSemis {
		t.Errorf("band between supported %.2fx (%s) and rejected %.2fx (%s) is only %.1f semitones wide; "+
			"want >= %.1f so a single threshold can separate them",
			supportedMax, supportedMaxID, rejectedMin, rejectedMinID, gapSemis, 2*minMarginSemis)
	}
	t.Logf("band: supported up to %.2fx (%s), rejected from %.2fx (%s), %.1f semitones apart",
		supportedMax, supportedMaxID, rejectedMin, rejectedMinID, gapSemis)
}
