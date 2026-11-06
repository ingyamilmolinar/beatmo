package fingerprint

import "testing"

func TestThresholdFileOverridesRegistryRow(t *testing.T) {
	r := NewRegistry()
	r.Register(Descriptor{ID: "t.x", Group: "g", Name: "x", Doc: "d", Kind: KindFraction,
		Drift: 0.05, Mismatch: 0.15, Applies: AlwaysApplies,
		Extract: func(*Bundle) Measurement { return Scalar(0) }})
	r.thresholdFile = ThresholdFile{"t.x": {Drift: 0.2, Mismatch: 0.4, N: 8, Source: "corpus"}}
	d, _ := r.ByID("t.x")
	if d.Drift != 0.2 || d.Mismatch != 0.4 {
		t.Fatalf("file must override: %+v", d)
	}
	if r.CalibratedFrom("t.x") != "corpus" {
		t.Fatal("CalibratedFrom must report the source")
	}
	// A row registered AFTER calibration is still overridden.
	r.thresholdFile["t.y"] = ThresholdEntry{Drift: 1, Mismatch: 2, N: 3, Source: "probe"}
	r.Register(Descriptor{ID: "t.y", Group: "g", Name: "y", Doc: "d", Kind: KindRatio,
		Applies: AlwaysApplies, Extract: func(*Bundle) Measurement { return Scalar(0) }})
	d, _ = r.ByID("t.y")
	if d.Drift != 1 || d.Mismatch != 2 {
		t.Fatalf("late registration must be calibrated too: %+v", d)
	}
}

func TestEmbeddedThresholdFileParses(t *testing.T) {
	_ = LoadThresholdFile()
}

// TestCalibratedFromIsEmptyForHandThresholds pins the report's third state:
// a descriptor with no file entry stays hand-thresholded and says so by
// reporting "" (which validate renders as "hand").
func TestCalibratedFromIsEmptyForHandThresholds(t *testing.T) {
	r := NewRegistry()
	r.Register(Descriptor{ID: "t.z", Group: "g", Name: "z", Doc: "d", Kind: KindFraction,
		Applies: AlwaysApplies, Extract: func(*Bundle) Measurement { return Scalar(0) }})
	if got := r.CalibratedFrom("t.z"); got != "" {
		t.Fatalf("CalibratedFrom(uncalibrated) = %q, want \"\"", got)
	}
}

// TestThresholdEntryWithZeroMismatchIsIgnored: an entry that would silence a
// descriptor entirely (mismatch 0 means "never fires") is a malformed row, not
// an instruction. The hand thresholds stand.
func TestThresholdEntryWithZeroMismatchIsIgnored(t *testing.T) {
	r := NewRegistry()
	r.Register(Descriptor{ID: "t.w", Group: "g", Name: "w", Doc: "d", Kind: KindFraction,
		Drift: 0.05, Mismatch: 0.15, Applies: AlwaysApplies,
		Extract: func(*Bundle) Measurement { return Scalar(0) }})
	r.thresholdFile = ThresholdFile{"t.w": {Drift: 0.2, Mismatch: 0, Source: "corpus"}}
	d, _ := r.ByID("t.w")
	if d.Drift != 0.05 || d.Mismatch != 0.15 {
		t.Fatalf("zero-mismatch entry must be ignored: %+v", d)
	}
	if r.CalibratedFrom("t.w") != "" {
		t.Fatal("an ignored entry must not be reported as calibrated")
	}
}
