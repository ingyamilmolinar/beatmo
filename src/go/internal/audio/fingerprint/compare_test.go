package fingerprint

import (
	"math"
	"regexp"
	"strings"
	"testing"
)

// The worked examples of the verdict-to-metric mapping: the moved classifiers behave identically.

func TestCompareSecondsRatio(t *testing.T) {
	// The mapping doc's harp example: 1.130s reference vs 0.160s synth = 7.1x short.
	d := CompareByKind(KindSeconds, Scalar(1.130), Scalar(0.160))
	if d.Verdict != VerdictMismatch {
		t.Errorf("verdict: got %v want MISMATCH", d.Verdict)
	}
	if d.Direction != "low" {
		t.Errorf("direction: got %q want low", d.Direction)
	}
	if d.Display != "7.1x short" {
		t.Errorf("display: got %q want %q", d.Display, "7.1x short")
	}
	if d.Norm < 1 {
		t.Errorf("Norm must be >=1 at or past mismatch; got %v", d.Norm)
	}
}

func TestCompareHzPercent(t *testing.T) {
	// The mapping doc calibrates +36% as drift, not mismatch.
	d := CompareByKind(KindHz, Scalar(1000), Scalar(1360))
	if d.Verdict != VerdictDrift {
		t.Errorf("verdict: got %v want drift", d.Verdict)
	}
	if d.Display != "+36%" {
		t.Errorf("display: got %q want %q", d.Display, "+36%")
	}
	if d.Norm >= 1 {
		t.Errorf("Norm must be <1 below mismatch; got %v", d.Norm)
	}
}

func TestCompareFractionAbsolute(t *testing.T) {
	// The mapping doc's worked example: delta +0.341 -> MISMATCH.
	d := CompareByKind(KindFraction, Scalar(0.100), Scalar(0.441))
	if d.Verdict != VerdictMismatch || d.Direction != "high" {
		t.Fatalf("got %v/%q want MISMATCH/high", d.Verdict, d.Direction)
	}
	if math.Abs(d.Signed-0.341) > 1e-9 {
		t.Errorf("Signed: got %v want 0.341", d.Signed)
	}
}

func TestCompareUnavailablePropagates(t *testing.T) {
	d := CompareByKind(KindFraction, Scalar(0.1), Unavailable("silent"))
	if d.OK {
		t.Error("comparing against an unavailable measurement must not be OK")
	}
	if d.NA != "silent" {
		t.Errorf("reason lost: %q", d.NA)
	}
	if d.Verdict != "" {
		t.Errorf("an unavailable comparison must not assert a verdict; got %q", d.Verdict)
	}
}

func TestCompareUnavailableReferencePropagates(t *testing.T) {
	d := CompareByKind(KindFraction, Unavailable("no plateau"), Scalar(0.1))
	if d.OK || d.NA != "no plateau" {
		t.Fatalf("got OK=%v NA=%q", d.OK, d.NA)
	}
}

// Signed holds the raw reduced magnitude, Norm it scaled by the mismatch threshold.
func TestCompareVectorL1Mean(t *testing.T) {
	d := CompareByKind(KindFraction,
		Vector([]float64{1, 0.5, 0.25}, nil),
		Vector([]float64{1, 0.5, 0.75}, nil))
	const wantMean = 0.5 / 3.0
	if math.Abs(d.Signed-wantMean) > 1e-9 {
		t.Errorf("Signed: got %v want %v", d.Signed, wantMean)
	}
	_, mismatch := KindFraction.Thresholds()
	if math.Abs(d.Norm-wantMean/mismatch) > 1e-9 {
		t.Errorf("Norm: got %v want %v", d.Norm, wantMean/mismatch)
	}
}

func TestCompareVectorIdenticalIsOK(t *testing.T) {
	v := []float64{1, 0.5, 0.25}
	d := CompareByKind(KindFraction, Vector(v, nil), Vector(v, nil))
	if d.Verdict != VerdictOK || d.Norm != 0 {
		t.Fatalf("identical vectors: got %v/%v want ok/0", d.Verdict, d.Norm)
	}
}

func TestCompareTrackResamplesToCommonLength(t *testing.T) {
	// The same ramp at two rates compares as near-identical, not a length mismatch.
	a := make([]float64, 100)
	for i := range a {
		a[i] = float64(i) / 99
	}
	b := make([]float64, 50)
	for i := range b {
		b[i] = float64(i) / 49
	}
	d := CompareByKind(KindFraction, Track(a, 0.01), Track(b, 0.02))
	if !d.OK {
		t.Fatalf("track comparison should be OK, got NA=%q", d.NA)
	}
	if d.Signed > 0.02 {
		t.Errorf("same ramp at two rates should compare near-zero; got RMS %v", d.Signed)
	}
}

func TestCompareIdentityIsZeroForEveryKind(t *testing.T) {
	for _, k := range []Kind{KindFraction, KindTinyFraction, KindSeconds, KindHz, KindSeriesDeviation, KindDB, KindRatio} {
		d := CompareByKind(k, Scalar(0.37), Scalar(0.37))
		if d.Verdict != VerdictOK {
			t.Errorf("%v: identity must be ok, got %v", k, d.Verdict)
		}
		if d.Direction != "" {
			t.Errorf("%v: identity must have no direction, got %q", k, d.Direction)
		}
	}
}

// A rate crossing zero (+57 vs -516 Hz/s) must not render as a percentage ("-1013%").
func TestCompareRateIsSignedNotPercentage(t *testing.T) {
	d := CompareWithThresholds(KindRate, Scalar(56.5), Scalar(-516.5), 150, 500)
	if d.Verdict != VerdictMismatch {
		t.Errorf("a sign reversal of this magnitude must be a mismatch; got %v", d.Verdict)
	}
	if d.Direction != "low" {
		t.Errorf("direction: got %q want low", d.Direction)
	}
	if math.Abs(d.Signed-(-573.0)) > 0.1 {
		t.Errorf("Signed: got %v want ~-573", d.Signed)
	}
	if strings.Contains(d.Display, "%") {
		t.Errorf("a rate must not render as a percentage; got %q", d.Display)
	}
}

// A reference near zero makes any percentage explode.
func TestCompareRateNearZeroReferenceIsFinite(t *testing.T) {
	d := CompareWithThresholds(KindRate, Scalar(0.0001), Scalar(-0.05), 0.02, 0.1)
	if math.IsNaN(d.Norm) || math.IsInf(d.Norm, 0) {
		t.Fatalf("Norm must stay finite near a zero reference; got %v", d.Norm)
	}
	if d.Verdict != VerdictDrift {
		t.Errorf("got %v want drift", d.Verdict)
	}
}

// KindRate spans Hz/s, dB/s and 1/s², orders of magnitude apart, so no default fits them all.
func TestDescriptorThresholdOverridesApply(t *testing.T) {
	base := Descriptor{ID: "t", Group: "g", Name: "n", Doc: "d", Kind: KindRate,
		Applies: AlwaysApplies, Extract: func(*Bundle) Measurement { return Scalar(0) }}

	// Without overrides KindRate's deliberately-wide defaults flag nothing.
	if got := base.CompareWith(Scalar(0), Scalar(100)).Verdict; got != VerdictOK {
		t.Errorf("un-scaled rate: got %v want ok (defaults must not flag)", got)
	}

	tight := base
	tight.Drift, tight.Mismatch = 10, 50
	if got := tight.CompareWith(Scalar(0), Scalar(100)).Verdict; got != VerdictMismatch {
		t.Errorf("scaled rate: got %v want MISMATCH", got)
	}

	d, m := tight.Thresholds()
	if d != 10 || m != 50 {
		t.Errorf("Thresholds(): got (%v,%v) want (10,50)", d, m)
	}
}

// TestKindDefaultsSurviveWhenNotOverridden guards the fallback half.
func TestKindDefaultsSurviveWhenNotOverridden(t *testing.T) {
	d := Descriptor{ID: "t", Group: "g", Name: "n", Doc: "d", Kind: KindFraction,
		Applies: AlwaysApplies, Extract: func(*Bundle) Measurement { return Scalar(0) }}
	gotD, gotM := d.Thresholds()
	wantD, wantM := KindFraction.Thresholds()
	if gotD != wantD || gotM != wantM {
		t.Errorf("got (%v,%v) want kind defaults (%v,%v)", gotD, gotM, wantD, wantM)
	}
}

// Identical in verdict, Norm and Mag; only the display handling differs.
func TestReducedDeltaDisplay_UsesPreformattedDisplayVerbatim(t *testing.T) {
	const mag = 1.28
	pre := reducedDeltaDisplay(mag, "mean 2.43x (worst 4k 6.52x)", 0.5, 1.0)
	if pre.Display != "mean 2.43x (worst 4k 6.52x)" {
		t.Errorf("preformatted display was modified: %q", pre.Display)
	}

	app := compareReduced(mag, "mean|Δ|", 0.5, 1.0)
	if app.Display != "mean|Δ| 1.280" {
		t.Errorf("compareReduced must still append the magnitude, got %q", app.Display)
	}

	// Same magnitude and thresholds => same classification either way.
	if pre.Verdict != app.Verdict || pre.Norm != app.Norm || pre.Mag != app.Mag ||
		pre.MagOK != app.MagOK || pre.Signed != app.Signed || pre.OK != app.OK {
		t.Errorf("the two entry points disagree beyond Display:\n pre=%+v\n app=%+v", pre, app)
	}
}

// The trailing number is the same quantity in another domain and reads as an extra column.
func TestPreformattedComparatorsDoNotDoublePrintTheMagnitude(t *testing.T) {
	r := DefaultRegistry()
	for _, tc := range []struct {
		id   string
		ref  Measurement
		syn  Measurement
		want string // substring the display must contain
	}{
		{IDBandT60,
			Vector([]float64{1, 2, 4}, octaveBandLabels[:3]),
			Vector([]float64{2, 8, 4}, octaveBandLabels[:3]), "x"},
		{IDDecayPerMode,
			Vector([]float64{-10, -20}, []string{"m1", "m2"}),
			Vector([]float64{-30, -25}, []string{"m1", "m2"}), "dB/s"},
		{IDModeRatios,
			Vector([]float64{0, 700, 1200}, []string{"m1", "m2", "m3"}),
			Vector([]float64{0, 900, 1500}, []string{"m1", "m2", "m3"}), "cents"},
	} {
		d, ok := r.ByID(tc.id)
		if !ok {
			t.Fatalf("%s not registered", tc.id)
		}
		got := d.CompareWith(tc.ref, tc.syn)
		if !got.OK {
			t.Fatalf("%s: comparator returned n/a: %s", tc.id, got.NA)
		}
		if !strings.Contains(got.Display, tc.want) {
			t.Errorf("%s: display %q lost its unit %q", tc.id, got.Display, tc.want)
		}
		// The double print ENDS in a bare three-decimal number.
		if m := trailingBareFloat.FindString(got.Display); m != "" {
			t.Errorf("%s: display %q ends in an appended bare magnitude %q", tc.id, got.Display, m)
		}
	}
}

var trailingBareFloat = regexp.MustCompile(` -?\d+\.\d{3}$`)
