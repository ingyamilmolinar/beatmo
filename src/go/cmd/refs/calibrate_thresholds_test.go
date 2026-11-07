//go:build !js

package main

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
)

// The injected resolver for the pure percentile tests; the production resolver is tested below and end to end.
func absSigned(a axisResult) (float64, bool) {
	if a.RefNA != "" || a.SynthNA != "" {
		return 0, false
	}
	return math.Abs(a.Signed), true
}

// goodSamples builds n good-graded samples carrying one axis row each.
func goodSamples(id string, mags ...float64) []validationSample {
	out := make([]validationSample, 0, len(mags))
	for _, m := range mags {
		out = append(out, validationSample{
			Class:     classGood,
			Diagnosis: instrumentDiagnosis{Axes: []axisResult{{ID: id, Name: id, Signed: m}}},
		})
	}
	return out
}

func TestGoodGroupThresholds_P50P90(t *testing.T) {
	samples := goodSamples("a.x", 1, 2, 3, 4, 10)
	samples = append(samples, validationSample{
		Class:     classBad,
		Diagnosis: instrumentDiagnosis{Axes: []axisResult{{ID: "a.x", Name: "a.x", Signed: 100}}},
	})
	f := goodGroupThresholds(samples, absSigned)
	e, ok := f["a.x"]
	if !ok {
		t.Fatalf("a.x not calibrated: %+v", f)
	}
	if e.N != 5 {
		t.Fatalf("N=%d, want 5 (the bad sample must not contribute)", e.N)
	}
	// Interpolating percentile over [1 2 3 4 10]: p50=3, p90=7.6.
	if math.Abs(e.Drift-3) > 1e-9 {
		t.Errorf("drift=%g, want p50=3", e.Drift)
	}
	if math.Abs(e.Mismatch-7.6) > 1e-9 {
		t.Errorf("mismatch=%g, want interpolated p90=7.6 (index rounding would give 10)", e.Mismatch)
	}
	if e.Source != "corpus" {
		t.Errorf("Source = %q, want corpus", e.Source)
	}
}

// TestGoodGroupThresholds_SkipsUnmeasured: an axis that was N/A on either side
// carries no magnitude, so it must not contribute a sample.
func TestGoodGroupThresholds_SkipsUnmeasured(t *testing.T) {
	ax := func(id string, signed float64, refNA, synthNA string) axisResult {
		return axisResult{ID: id, Name: id, Signed: signed, RefNA: refNA, SynthNA: synthNA}
	}
	var samples []validationSample
	for i, m := range []float64{1, 2, 3, 4, 5} {
		refNA, synthNA := "", ""
		if i%2 == 0 {
			refNA = "no f0"
		} else {
			synthNA = "no f0"
		}
		samples = append(samples, validationSample{Class: classGood, Diagnosis: instrumentDiagnosis{
			Axes: []axisResult{ax("a.x", m, "", ""), ax("a.na", m, refNA, synthNA)},
		}})
	}
	f := goodGroupThresholds(samples, absSigned)
	if _, ok := f["a.na"]; ok {
		t.Error("an axis unavailable on every good sample must not be calibrated")
	}
	if _, ok := f["a.x"]; !ok {
		t.Error("a.x must be calibrated")
	}
}

// TestGoodGroupThresholds_UsesMagnitude: a symmetric spread around zero must
// not average itself into a threshold of zero.
func TestGoodGroupThresholds_UsesMagnitude(t *testing.T) {
	f := goodGroupThresholds(goodSamples("a.x", -4, -2, 2, 4, -6), absSigned)
	e := f["a.x"]
	if e.Drift < 2 || e.Mismatch < e.Drift {
		t.Fatalf("magnitude-based thresholds expected, got %+v", e)
	}
}

// TestGoodGroupThresholds_BelowFloor: four good families cannot support a p90.
// The floor is 5; the axis keeps its hand threshold.
func TestGoodGroupThresholds_BelowFloor(t *testing.T) {
	if f := goodGroupThresholds(goodSamples("a.x", 1, 2, 3, 4), absSigned); len(f) != 0 {
		t.Fatalf("n=4 is below goodGroupCalibrationMinN=%d and must not calibrate, got %+v",
			goodGroupCalibrationMinN, f)
	}
	if f := goodGroupThresholds(goodSamples("a.x", 1, 2, 3, 4, 5), absSigned); len(f) != 1 {
		t.Fatalf("n=5 is at the floor and must calibrate, got %+v", f)
	}
}

// A zero p50 would read as "no override" and could invert the two bands.
func TestGoodGroupThresholds_DriftIsAlwaysPositive(t *testing.T) {
	e := goodGroupThresholds(goodSamples("a.x", 0, 0, 0, 0, 5), absSigned)["a.x"]
	if e.Drift <= 0 {
		t.Fatalf("drift must be positive even when p50 is 0: %+v", e)
	}
	if e.Drift >= e.Mismatch {
		t.Fatalf("drift %g must stay below mismatch %g", e.Drift, e.Mismatch)
	}
}

// --- the production resolver -------------------------------------------------

// The REAL comparator's magnitude must be the quantity it weighs: a folded RATIO for KindSeconds (0.01s vs 0.03s is
// 3x, not 0.02), a PERCENTAGE for KindHz (1000 vs 1400 Hz is 40 %). Expectations come from the definition, not from
// d.Thresholds(), which would be a tautology.
func TestRegistryAxisMagnitude_RecoversTheComparedQuantity(t *testing.T) {
	cases := []struct {
		id         string
		ref, synth float64
		wantMag    float64
		what       string
	}{
		{fingerprint.IDEnvAttack, 0.01, 0.03, 3, "folded ratio (seconds)"},
		{fingerprint.IDEnvAttack, 0.03, 0.01, 3, "folded ratio, short direction"},
		{fingerprint.IDCentroid, 1000, 1400, 40, "percentage of the reference (hz)"},
		{fingerprint.IDCentroid, 1000, 600, 40, "percentage, low direction"},
	}
	for _, c := range cases {
		d, ok := fingerprint.DefaultRegistry().ByID(c.id)
		if !ok {
			t.Fatalf("descriptor %q missing", c.id)
		}
		delta := d.CompareWith(fingerprint.Scalar(c.ref), fingerprint.Scalar(c.synth))
		got, ok := registryAxisMagnitude(axisResult{
			ID: c.id, Norm: delta.Norm, Signed: delta.Signed,
			Mag: delta.Mag, MagOK: delta.MagOK, Delta: delta.Display,
		})
		if !ok {
			t.Errorf("%s (%v -> %v): magnitude refused", c.id, c.ref, c.synth)
			continue
		}
		if math.Abs(got-c.wantMag) > 1e-9 {
			t.Errorf("%s (%v -> %v): magnitude=%g, want %g -- %s",
				c.id, c.ref, c.synth, got, c.wantMag, c.what)
		}
		if math.Abs(got-math.Abs(delta.Signed)) < 1e-9 && c.wantMag != math.Abs(delta.Signed) {
			t.Errorf("%s: magnitude collapsed to |Signed|=%g", c.id, delta.Signed)
		}
	}
}

// band.t60 DECLARES T60 ratios and COMPARES mean|log2 ratio|: the magnitude must come back as a ratio.
func TestRegistryAxisMagnitude_TransformingComparator(t *testing.T) {
	d, ok := fingerprint.DefaultRegistry().ByID(fingerprint.IDBandT60)
	if !ok || d.Compare == nil {
		t.Skip("band.t60 no longer has a custom comparator")
	}
	// Every band exactly 4x longer on the synth: mean|log2| = 2, ratio = 4.
	ref := make([]float64, 8)
	synth := make([]float64, 8)
	for i := range ref {
		ref[i], synth[i] = 0.5, 2.0
	}
	delta := d.CompareWith(fingerprint.Vector(ref, nil), fingerprint.Vector(synth, nil))
	got, ok := registryAxisMagnitude(axisResult{
		ID: d.ID, Norm: delta.Norm, Signed: delta.Signed,
		Mag: delta.Mag, MagOK: delta.MagOK, Delta: delta.Display,
	})
	if !ok {
		t.Fatalf("band.t60 magnitude refused (delta=%+v)", delta)
	}
	if math.Abs(got-4) > 1e-9 {
		t.Fatalf("band.t60 magnitude=%g, want the 4x RATIO -- Norm x mismatch would give %g",
			got, delta.Norm*2.0)
	}
}

// Each would otherwise write an inert entry while the report's CAL column claimed it was earned from the corpus.
func TestRegistryAxisMagnitude_RefusesUncalibratable(t *testing.T) {
	d, ok := fingerprint.DefaultRegistry().ByID(fingerprint.IDEnvRingT60)
	if !ok || !d.Demoted {
		t.Skip("ring T60 is no longer demoted")
	}
	if _, ok := registryAxisMagnitude(axisResult{ID: d.ID, Mag: 1.5, MagOK: true}); ok {
		t.Error("a Demoted descriptor is excluded from the trusted set and must not be calibrated")
	}
	if _, ok := registryAxisMagnitude(axisResult{ID: fingerprint.IDEnvAttack, Mag: 1, MagOK: false}); ok {
		t.Error("a sentinel row (MagOK false) carries no measurement and must be refused")
	}
	if _, ok := registryAxisMagnitude(axisResult{ID: fingerprint.IDEnvAttack, Mag: 1, MagOK: true, RefNA: "silent"}); ok {
		t.Error("an unavailable measurement must be refused")
	}
	if _, ok := registryAxisMagnitude(axisResult{ID: "no.such.id", Mag: 1, MagOK: true}); ok {
		t.Error("an unknown descriptor must be refused")
	}
	if _, ok := registryAxisMagnitude(axisResult{ID: fingerprint.IDEnvAttack, Mag: math.NaN(), MagOK: true}); ok {
		t.Error("a NaN magnitude must be refused")
	}
}

// The whole command path: the resolver and the statistics tested apart miss a unit error in the seam between them.
func TestGoodGroupThresholds_WithProductionResolver(t *testing.T) {
	reg := fingerprint.DefaultRegistry()
	attack, ok := reg.ByID(fingerprint.IDEnvAttack)
	if !ok {
		t.Fatal("env.attack missing")
	}
	centroid, ok := reg.ByID(fingerprint.IDCentroid)
	if !ok {
		t.Fatal("bright.centroid missing")
	}

	// Five good families. Attack ratios 1.5, 2, 2.5, 3, 4; centroid +10%,
	// +20%, +30%, +40%, +50%.
	ratios := []float64{1.5, 2, 2.5, 3, 4}
	pcts := []float64{10, 20, 30, 40, 50}
	var samples []validationSample
	for i := range ratios {
		row := func(d fingerprint.Descriptor, ref, synth float64) axisResult {
			dd := d.CompareWith(fingerprint.Scalar(ref), fingerprint.Scalar(synth))
			return axisResult{ID: d.ID, Name: d.Name, Group: d.Group, Norm: dd.Norm,
				Signed: dd.Signed, Mag: dd.Mag, MagOK: dd.MagOK, Delta: dd.Display}
		}
		samples = append(samples, validationSample{Class: classGood, Diagnosis: instrumentDiagnosis{
			Axes: []axisResult{
				row(attack, 0.02, 0.02*ratios[i]),
				row(centroid, 1000, 1000*(1+pcts[i]/100)),
			},
		}})
	}

	f := goodGroupThresholds(samples, registryAxisMagnitude)

	ea, ok := f[fingerprint.IDEnvAttack]
	if !ok {
		t.Fatalf("env.attack not calibrated: %+v", f)
	}
	// p50 of [1.5 2 2.5 3 4] = 2.5; p90 = 3.6.
	if math.Abs(ea.Drift-2.5) > 1e-9 || math.Abs(ea.Mismatch-3.6) > 1e-9 {
		t.Errorf("env.attack: drift=%g mismatch=%g, want 2.5 / 3.6 (folded RATIOS, not seconds)",
			ea.Drift, ea.Mismatch)
	}
	ec, ok := f[fingerprint.IDCentroid]
	if !ok {
		t.Fatalf("bright.centroid not calibrated: %+v", f)
	}
	// p50 of [10 20 30 40 50] = 30; p90 = 46.
	if math.Abs(ec.Drift-30) > 1e-9 || math.Abs(ec.Mismatch-46) > 1e-9 {
		t.Errorf("bright.centroid: drift=%g mismatch=%g, want 30 / 46 (PERCENT, not hertz)",
			ec.Drift, ec.Mismatch)
	}

	// And the calibrated pair must actually classify: an instrument at the
	// good group's p50 drifts, one past p90 mismatches.
	tuned := attack
	tuned.Drift, tuned.Mismatch = ea.Drift, ea.Mismatch
	if v := tuned.CompareWith(fingerprint.Scalar(0.02), fingerprint.Scalar(0.02*2.6)).Verdict; v != "drift" {
		t.Errorf("a 2.6x attack against a p50 of 2.5 should drift, got %q", v)
	}
	if v := tuned.CompareWith(fingerprint.Scalar(0.02), fingerprint.Scalar(0.02*5)).Verdict; v != "MISMATCH" {
		t.Errorf("a 5x attack against a p90 of 3.6 should mismatch, got %q", v)
	}
}

// --- pinned entries -----------------------------------------------------------

func TestMergeProbeEntriesWins(t *testing.T) {
	fresh := thresholdFileOf(map[string][2]float64{"a.x": {1, 2}})
	existing := thresholdFileOf(map[string][2]float64{"a.x": {9, 9}, "a.y": {5, 6}})
	for id, e := range existing {
		e.Source = "probe"
		existing[id] = e
	}
	got := mergeProbeEntries(fresh, existing)
	if got["a.x"].Drift != 9 {
		t.Errorf("probe entry must win over the corpus: %+v", got["a.x"])
	}
	if got["a.y"].Drift != 5 {
		t.Errorf("probe-only entry must survive: %+v", got["a.y"])
	}
	if got["a.x"].Source != "probe" {
		t.Errorf("Source must stay probe: %+v", got["a.x"])
	}
}

// A hand pin is an edit to the JSON; recalibration must carry it over WITHOUT a rebuild.
func TestHandPinOnDiskSurvivesRecalibration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "thresholds.json")
	pinned := fingerprint.ThresholdFile{
		"env.attack": {Drift: 1.25, Mismatch: 1.75, N: 0, Source: "hand",
			Note: "corpus p90 flipped the harp's ring complaint to ok"},
	}
	raw, err := json.MarshalIndent(pinned, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	// The embedded copy must NOT be what gets merged: prove the on-disk pin
	// is picked up even though this binary embeds a different file.
	existing, err := readThresholdFileAt(path)
	if err != nil {
		t.Fatalf("readThresholdFileAt: %v", err)
	}
	fresh := thresholdFileOf(map[string][2]float64{"env.attack": {9, 9}, "a.other": {1, 2}})
	got := mergeProbeEntries(fresh, existing)

	e := got["env.attack"]
	if e.Source != "hand" || e.Drift != 1.25 || e.Mismatch != 1.75 {
		t.Fatalf("hand pin on disk did not survive recalibration: %+v", e)
	}
	if e.Note == "" {
		t.Error("a pinned entry must carry its reason; an unexplained pin is indistinguishable from a stale one")
	}
	if _, ok := got["a.other"]; !ok {
		t.Error("unpinned fresh entries must still be written")
	}
	if _, ok := fingerprint.LoadThresholdFile()["env.attack"]; ok {
		t.Log("note: the embedded file also has env.attack; the on-disk value above is what won")
	}
}

func TestReadThresholdFileAt_MissingIsEmptyNotAnError(t *testing.T) {
	got, err := readThresholdFileAt(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("a missing file is the first run, not an error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want empty, got %+v", got)
	}
}

func TestReadThresholdFileAt_MalformedIsAnError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "thresholds.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readThresholdFileAt(path); err == nil {
		t.Fatal("a malformed calibration must not be silently treated as empty -- " +
			"that would drop every pinned entry on the next write")
	}
}

// thresholdFileOf builds a ThresholdFile from id -> {drift, mismatch}.
func thresholdFileOf(m map[string][2]float64) fingerprint.ThresholdFile {
	out := fingerprint.ThresholdFile{}
	for id, dm := range m {
		out[id] = fingerprint.ThresholdEntry{Drift: dm[0], Mismatch: dm[1], N: 5, Source: "corpus"}
	}
	return out
}

// A degenerate good group's mismatch is 1.5x the drift, not a measurement; the Note is how a reader tells two
// identical-looking entries apart.
func TestCalibrate_SynthesisedMismatchIsRecorded(t *testing.T) {
	// Six good families, all reading exactly the same magnitude: p50 == p90.
	var samples []validationSample
	for i := 0; i < 6; i++ {
		samples = append(samples, validationSample{Class: classGood, Diagnosis: instrumentDiagnosis{
			Axes: []axisResult{
				{ID: "flat.axis", Name: "flat.axis", Signed: 2.0},
				// A second axis with real spread, as the control.
				{ID: "spread.axis", Name: "spread.axis", Signed: float64(i) + 1},
			},
		}})
	}
	f := goodGroupThresholds(samples, absSigned)

	flat, ok := f["flat.axis"]
	if !ok {
		t.Fatal("flat.axis was not calibrated")
	}
	if flat.Mismatch <= flat.Drift {
		t.Fatalf("degenerate group must still spread the bands: %+v", flat)
	}
	want := "p90 <= p50 over n=6; mismatch synthesised as 1.5*drift"
	if flat.Note != want {
		t.Errorf("Note = %q, want %q", flat.Note, want)
	}
	if flat.Source != "corpus" {
		t.Errorf("Source = %q; the drift half IS the corpus p50, so the entry stays a corpus entry", flat.Source)
	}

	spread, ok := f["spread.axis"]
	if !ok {
		t.Fatal("spread.axis was not calibrated")
	}
	if spread.Note != "" {
		t.Errorf("an honestly-measured p90 must carry no synthesis note, got %q", spread.Note)
	}
}

// A zero p50 leaves no measured drift, so it is derived as mismatch/1.5 and noted.
func TestCalibrate_SynthesisedDriftIsRecorded(t *testing.T) {
	// Five good families: three read exactly 0, the top two read high, so
	// p50 == 0 while p90 is a real number.
	mags := []float64{0, 0, 0, 0.4, 0.6}
	var samples []validationSample
	for _, m := range mags {
		samples = append(samples, validationSample{Class: classGood, Diagnosis: instrumentDiagnosis{
			Axes: []axisResult{{ID: "zero.p50", Name: "zero.p50", Signed: m}},
		}})
	}
	e, ok := goodGroupThresholds(samples, absSigned)["zero.p50"]
	if !ok {
		t.Fatal("zero.p50 was not calibrated")
	}
	if e.Drift <= 0 {
		t.Fatalf("drift must be anchored below mismatch, got %+v", e)
	}
	want := "p50 == 0 over n=5; drift synthesised as mismatch/1.5"
	if e.Note != want {
		t.Errorf("Note = %q, want %q", e.Note, want)
	}
}
