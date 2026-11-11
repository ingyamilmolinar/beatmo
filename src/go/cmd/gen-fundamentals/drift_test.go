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

// TestInstrumentFundamentalsUpToDate re-measures every instrument and fails
// when the committed table no longer matches.
//
// This is the whole answer to "what happens when an instrument gets retuned?".
// The table records what each voice SOUNDED at generation time, so a seed
// change that moves a fundamental makes it stale — and a stale table is a wrong
// note name, silently, on both platforms. Rather than recomputing at runtime
// (impossible in the browser: the C DSP is Emscripten-side, so Go on wasm never
// holds PCM), the staleness is made impossible to ignore: this fails the build
// and `make gen-fundamentals` fixes it, so a retune is caught in review.
//
// It compares the RENDERED SOURCE rather than the parsed values so that a
// change to the generator's output — a new field, a different rounding — is
// caught too, matching how the repo's other *.gen.go files are guarded.
func TestInstrumentFundamentalsUpToDate(t *testing.T) {
	if renderIsStub {
		// Deliberately a visible skip and not a build tag. Under `-tags test`
		// synthmatch renders a deterministic pure-Go stub, so re-measuring here
		// would compare the real committed values against synthetic ones and
		// fail for no reason. Build-tagging the file out instead would make
		// `go test -tags test ./...` report a silent pass — which is exactly how
		// a byte-identity guard in this repo went unnoticed for a whole phase.
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

// TestAgreementBandIsCalibrated pins the empty band agreementToleranceSemis
// sits in, so a DSP change that narrows it fails here rather than silently
// reclassifying an instrument.
//
// The guard distinguishes a legitimate pitch-envelope drop (a recipe whose
// `pitch` post stage bends the voice below its oscillator setting) from an
// octave-class detector error. Those two populations must stay separated by a
// wide, empty margin for a single threshold to tell them apart at all.
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

// TestF0SupportBandIsCalibrated pins the empty band that
// fingerprint.f0SupportMaxPartialRatio sits inside, the same way
// TestAgreementBandIsCalibrated pins the agreement tolerance.
//
// The gate separates two populations by where the spectrum's lowest significant
// partial sits relative to the claimed f0: a real pitch has one within its first
// few harmonics (even a missing-fundamental voice, at ~3x), while a cymbal or a
// hi-hat has none until 6x or beyond. A single threshold can only tell them
// apart while that gap stays wide and empty, so the gap is asserted rather than
// assumed — a DSP change that narrows it fails here instead of silently
// renaming an instrument's note or taking one away.
func TestF0SupportBandIsCalibrated(t *testing.T) {
	if renderIsStub {
		t.Skip("render path is the -tags test stub; run without -tags test (needs CGO + build/libdrums.a)")
	}

	// Widest lowest-partial ratio among instruments we DO name a note for,
	// and narrowest among those we do not. The threshold must sit between them
	// with real margin on each side.
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
