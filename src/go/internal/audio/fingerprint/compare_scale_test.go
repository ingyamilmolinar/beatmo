package fingerprint

import "testing"

// A vector's mean |Δ| is in the VECTOR'S units, so KindFraction's thresholds are nonsense for hertz or cents: two
// centroid trajectories 45 Hz apart scored MISMATCH at Norm 300.

func TestCompareVector_HonoursDescriptorThresholds(t *testing.T) {
	// A vector of hertz, with thresholds declared in hertz.
	d := Descriptor{
		ID: "probe.hz_vector", Group: "g", Name: "n", Doc: "d",
		Kind: KindHz, Drift: 200, Mismatch: 800,
		Applies: AlwaysApplies,
		Extract: func(*Bundle) Measurement { return Scalar(0) },
	}
	close := d.CompareWith(
		Vector([]float64{1400, 1600, 1800, 2070}, nil),
		Vector([]float64{1450, 1650, 1850, 2100}, nil)) // mean |Δ| = 45 Hz
	if close.Verdict != VerdictOK {
		t.Errorf("trajectories 45 Hz apart must be ok on a 200/800 Hz scale, got %q (norm=%.2f)",
			close.Verdict, close.Norm)
	}
	if close.Norm >= 1 {
		t.Errorf("Norm must stay below 1 for an ok verdict, got %.2f", close.Norm)
	}

	far := d.CompareWith(
		Vector([]float64{1400, 1600, 1800, 2070}, nil),
		Vector([]float64{3000, 3200, 3400, 3670}, nil)) // mean |Δ| = 1600 Hz
	if far.Verdict != VerdictMismatch {
		t.Errorf("trajectories 1600 Hz apart must be a MISMATCH on a 200/800 Hz scale, got %q", far.Verdict)
	}
}

func TestCompareTrack_HonoursDescriptorThresholds(t *testing.T) {
	d := Descriptor{
		ID: "probe.hz_track", Group: "g", Name: "n", Doc: "d",
		Kind: KindHz, Drift: 200, Mismatch: 800,
		Applies: AlwaysApplies,
		Extract: func(*Bundle) Measurement { return Scalar(0) },
	}
	got := d.CompareWith(
		Track([]float64{1400, 1600, 1800}, 0.01),
		Track([]float64{1450, 1650, 1850}, 0.01)) // rms |Δ| = 50 Hz
	if got.Verdict != VerdictOK {
		t.Errorf("tracks 50 Hz apart must be ok on a 200/800 Hz scale, got %q (norm=%.2f)", got.Verdict, got.Norm)
	}
}

// Unit-fraction profiles are unaffected.
func TestCompareVector_DefaultsToFractionScale(t *testing.T) {
	got := CompareByKind(KindFraction,
		Vector([]float64{0.10, 0.20}, nil),
		Vector([]float64{0.11, 0.21}, nil)) // mean |Δ| = 0.01
	if got.Verdict != VerdictOK {
		t.Errorf("fraction profile 0.01 apart must be ok, got %q", got.Verdict)
	}
	big := CompareByKind(KindFraction,
		Vector([]float64{0.10, 0.20}, nil),
		Vector([]float64{0.40, 0.50}, nil)) // mean |Δ| = 0.30
	if big.Verdict != VerdictMismatch {
		t.Errorf("fraction profile 0.30 apart must be a MISMATCH, got %q", big.Verdict)
	}
}

// synth.Scalar is structurally 0 for a vector: a confident MISMATCH from a value never measured.
func TestCompare_FormMismatchIsUnavailable(t *testing.T) {
	got := CompareByKind(KindHz, Scalar(1500), Vector([]float64{1500, 1600}, nil))
	if got.OK {
		t.Errorf("a scalar/vector form mismatch must not report OK, got verdict=%q signed=%.1f", got.Verdict, got.Signed)
	}
	if got.Verdict != "" {
		t.Errorf("a form mismatch must assert no verdict, got %q", got.Verdict)
	}
	if got.NA == "" {
		t.Error("a form mismatch must explain itself via NA")
	}
	t.Logf("NA = %q", got.NA)

	rev := CompareByKind(KindHz, Vector([]float64{1500, 1600}, nil), Scalar(1500))
	if rev.OK {
		t.Errorf("the reverse form mismatch must not report OK either, got verdict=%q", rev.Verdict)
	}
}

// Fixed component positions: a short vector must not panic the report.
func TestCompareRingT60_ShortVectorIsUnavailable(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("compareRingT60 panicked on a short vector: %v", r)
		}
	}()
	rtDrift, rtMismatch := ringT60Thresholds()
	got := compareRingT60(Vector([]float64{1.0}, nil), Vector([]float64{1.0, 0, -20}, nil), rtDrift, rtMismatch)
	if got.OK {
		t.Errorf("a malformed ring-T60 measurement must not report OK, got %+v", got)
	}
}
