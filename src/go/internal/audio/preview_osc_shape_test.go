//go:build test

package audio

import "testing"

// TestPreviewOscShape_EveryEnumValueDistinctFromSine is the coverage gate over
// previewOscShape's oscillator table: EVERY osc_type enum value the modular
// recipe offers must render a shape DIFFERENT from Sine (value 0). A value that
// falls through previewOscShape's `default` case silently renders as a sine, so
// the Synth tab's "Oscillator" dropdown would show the user an identical picture
// for two different oscillators — the exact drift that let osc_type 12 ("Voice",
// the Rosenberg glottal pulse) ship rendering as a plain sine.
//
// Carve-out: value 4 ("FM") is NOT checked here. previewOscShape deliberately
// leaves FM to its caller (previewFMSample applies the operator matrix on top of
// the base shape), so with no FM depth configured a bare osc_type==4 IS a sine
// by design — see previewOscShape's doc comment.
//
// This complements TestPreviewWave_RespondsToWaveShapeEveryRecipe, which only
// exercises Sine→Saw (0→1) and therefore cannot see a stale high enum value.
func TestPreviewOscShape_EveryEnumValueDistinctFromSine(t *testing.T) {
	var oscDef ParamDef
	for _, d := range ModularSynthParamDefs() {
		if d.Name == "osc_type" {
			oscDef = d
			break
		}
	}
	if oscDef.Name == "" {
		t.Fatal("osc_type not found in ModularSynthParamDefs — schema drift")
	}
	if len(oscDef.Enum) == 0 {
		t.Fatal("osc_type carries no Enum labels — cannot enumerate oscillators")
	}
	// The enum must cover the declared range, or values are unreachable in the UI.
	if got, want := len(oscDef.Enum), int(oscDef.Max-oscDef.Min)+1; got != want {
		t.Fatalf("osc_type enum/range drift: %d labels for range [%g,%g] (want %d labels)",
			got, oscDef.Min, oscDef.Max, want)
	}

	// Sample one full cycle at a fixed resolution; hash the shape.
	shapeHash := func(oscType int) uint64 {
		const pts = 256
		buf := make([]float64, pts)
		for i := range buf {
			buf[i] = previewOscShape(oscType, float64(i)/float64(pts))
		}
		return previewHash(buf)
	}

	sine := shapeHash(0)
	for v := int(oscDef.Min); v <= int(oscDef.Max); v++ {
		if v == 0 {
			continue
		}
		if v == 4 { // FM: base shape is a sine by design; the caller adds the operators.
			continue
		}
		if shapeHash(v) == sine {
			t.Errorf("osc_type %d (%q): previewOscShape renders IDENTICAL to Sine — "+
				"the oscillator falls through to the default sine case, so the Synth tab "+
				"draws the same picture for two different oscillators",
				v, oscDef.Enum[v])
		}
	}
}

// TestPreviewOscShape_BowKnobsReshapeBowedString pins the bowed-string model's
// contract in BOTH directions:
//
//   - every osc_bow_* knob must reshape the Bowed String oscillator (osc_type 7),
//     so the Synth tab's focus graph can honestly explain the knob the user is
//     dragging on the violin/cello voices;
//   - and it must NOT reshape any other oscillator, because a bow param is
//     waveguide-internal to osc_type 7 — a picture that moved on a Sine voice
//     would be a lie (those recipes correctly hide the knob entirely; see
//     synthOscModelParamHidden in internal/ui).
func TestPreviewOscShape_BowKnobsReshapeBowedString(t *testing.T) {
	const bowedString = 7
	shapeHash := func(oscType int, m OscModelParams) uint64 {
		const pts = 256
		buf := make([]float64, pts)
		for i := range buf {
			buf[i] = previewOscShapeModel(oscType, float64(i)/float64(pts), m)
		}
		return previewHash(buf)
	}
	// Min/Max mirror the osc_bow_* ParamDef bounds in ModularSynthParamDefs.
	knobs := []struct {
		name     string
		min, max float64
		set      func(*OscModelParams, float64)
	}{
		{"osc_bow_pos", 0.02, 0.4, func(m *OscModelParams, v float64) { m.BowPos = v }},
		{"osc_bow_slope", 1.0, 6.0, func(m *OscModelParams, v float64) { m.BowSlope = v }},
		{"osc_bow_vel", 0.05, 0.6, func(m *OscModelParams, v float64) { m.BowVel = v }},
		{"osc_bow_loss", 0.2, 0.9, func(m *OscModelParams, v float64) { m.BowLoss = v }},
	}
	for _, k := range knobs {
		lo, hi := DefaultOscModelParams(), DefaultOscModelParams()
		k.set(&lo, k.min)
		k.set(&hi, k.max)

		if shapeHash(bowedString, lo) == shapeHash(bowedString, hi) {
			t.Errorf("%s: Bowed String shape IDENTICAL at Min(%g) and Max(%g) — "+
				"the preview ignores this bow knob", k.name, k.min, k.max)
		}
		// Honesty: a bow knob must be inert on every non-bowed oscillator.
		for _, other := range []int{0, 1, 2, 3, 9, 11, 12} {
			if shapeHash(other, lo) != shapeHash(other, hi) {
				t.Errorf("%s: changed the osc_type %d shape — bow params are internal to "+
					"the Bowed String waveguide, so this picture would lie", k.name, other)
			}
		}
	}
}
