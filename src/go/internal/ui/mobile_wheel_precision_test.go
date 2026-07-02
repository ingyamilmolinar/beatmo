// src/go/internal/ui/mobile_wheel_precision_test.go
package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// openWheelForPrecision builds a continuous (endless) wheel popup over a single
// unitless param, value seeded low so there is headroom to nudge upward.
func openWheelForPrecision(t *testing.T, def audio.ParamDef, value, stepMul float64) (*MobileWheelPopup, *Knob) {
	t.Helper()
	k := &Knob{Endless: true}
	k.Scale = KnobScale{Min: def.Min, Max: def.Max, Unit: def.Unit}
	k.Value = value
	k.StepMul = stepMul
	b := WheelBinding{Knob: k, Def: def, OnChange: func() {}, Title: func() string { return def.Name }}
	w := NewMobileWheelPopup()
	w.Open(b, image.Rect(0, 0, 10, 10), image.Rect(0, 0, 400, 800), 0)
	return w, k
}

// TestKnobPillLabelTracksResolution covers the inline mobile knob pill (the
// resting-state face of the control, tapped to open the wheel): it must show the
// same resolution-aware precision as the wheel center, so a value dialed in at a
// fine resolution is still legible after the wheel closes.
func TestKnobPillLabelTracksResolution(t *testing.T) {
	def := audio.ParamDef{Name: "amt", Min: 0, Max: 1} // unitless
	k := &Knob{Endless: true}
	k.Scale = KnobScale{Min: 0, Max: 1}
	k.Value = 0.4
	k.StepMul = 0.5 // coarse
	if got := knobPillLabel(k, def); got != "0.4" {
		t.Fatalf("coarse pill: got %q want %q", got, "0.4")
	}
	k.StepMul = 0.005 // fine -> 3 decimals
	if got := knobPillLabel(k, def); got != "0.400" {
		t.Fatalf("fine pill: got %q want %q", got, "0.400")
	}
}

func TestStepDecimals(t *testing.T) {
	cases := []struct {
		step float64
		want int
	}{
		{0.0005, 4}, {0.001, 3}, {0.002, 3}, {0.005, 3},
		{0.01, 2}, {0.02, 2}, {0.05, 2},
		{0.1, 1}, {0.2, 1}, {0.5, 1},
		{1, 0}, {2, 0}, {5, 0}, {10, 0}, {100, 0},
		{0, 0}, {-1, 0},
	}
	for _, c := range cases {
		if got := stepDecimals(c.step); got != c.want {
			t.Errorf("stepDecimals(%v) = %d, want %d", c.step, got, c.want)
		}
	}
}

// TestFormatParamValueStepFineResolution pins precision + sign handling per unit
// at a fine step. Each row would render with too few decimals (frozen) under the
// natural-precision formatter.
func TestFormatParamValueStepFineResolution(t *testing.T) {
	cases := []struct {
		value, step float64
		unit        string
		want        string
	}{
		{0.4, 0.005, "", "0.400"},
		{1.0, 0.005, "×", "1.000×"},
		{0.2, 0.001, "s", "0.200 s"},
		{2008, 10, "Hz", "2.01 kHz"},
		{440, 0.5, "Hz", "440.0 Hz"},
		{-3.5, 0.1, "st", "-3.5 st"},
		{0, 0.1, "st", "+0.0 st"},
	}
	for _, c := range cases {
		if got := formatParamValueStep(c.value, c.step, c.unit); got != c.want {
			t.Errorf("formatParamValueStep(%v, %v, %q) = %q, want %q", c.value, c.step, c.unit, got, c.want)
		}
	}
}

// TestFormatParamValueStepCoarseMatchesNatural guarantees that a coarse/infinite
// step reproduces the natural-precision formatter byte-for-byte, so existing
// captions are untouched.
func TestFormatParamValueStepCoarseMatchesNatural(t *testing.T) {
	units := []string{"", "×", "x", "mult", "Hz", "s", "ms", "cents", "c", "st", "dB", "%", "rad"}
	values := []float64{0, 0.4, 1, 12.5, -7.25, 1500, 440}
	for _, u := range units {
		for _, v := range values {
			want := formatParamValue(v, u)
			if got := formatParamValueStep(v, 1e9, u); got != want {
				t.Errorf("coarse formatParamValueStep(%v, 1e9, %q) = %q, want natural %q", v, u, got, want)
			}
		}
	}
}

// TestWheelCenterValueChangesOnFineResolutionStep is the user-reported scenario:
// at a fine resolution (x0.005) a single-step nudge must visibly change the
// number drawn in the center selector. Before the fix the center label kept its
// 1-decimal precision and showed the same string after a 0.005 change.
func TestWheelCenterValueChangesOnFineResolutionStep(t *testing.T) {
	def := audio.ParamDef{Name: "amt", Min: 0, Max: 1} // unitless
	w, k := openWheelForPrecision(t, def, 0.1, 0.005)

	before := w.liveValueLabel()
	k.NudgeEndless(knobEndlessPxPerNotch) // exactly one step == +0.005
	after := w.liveValueLabel()
	if before == after {
		t.Fatalf("center value must change after a one-step nudge at x0.005; stayed %q", before)
	}
}

// TestWheelCenterValuePrecisionTracksResolution pins the exact rendered strings:
// a coarse step keeps the natural 1-decimal precision, a fine step gains the
// decimals needed to reflect the resolution.
func TestWheelCenterValuePrecisionTracksResolution(t *testing.T) {
	def := audio.ParamDef{Name: "amt", Min: 0, Max: 1} // unitless
	w, k := openWheelForPrecision(t, def, 0.4, 0.5)

	if got := w.liveValueLabel(); got != "0.4" {
		t.Fatalf("coarse resolution: got %q want %q", got, "0.4")
	}
	k.StepMul = 0.005 // fine resolution -> 3 decimals
	if got := w.liveValueLabel(); got != "0.400" {
		t.Fatalf("fine resolution: got %q want %q", got, "0.400")
	}
}

// TestWheelCenterValueRefreshesAtEveryResolution walks every rung of the
// resolution ladder (the full set of supported resolutions) and asserts that a
// single-step nudge changes the rendered center number at each one. This is the
// core invariant: no supported resolution may leave the displayed value frozen
// after a real value change.
func TestWheelCenterValueRefreshesAtEveryResolution(t *testing.T) {
	defs := []audio.ParamDef{
		{Name: "amt", Min: 0, Max: 1},               // unitless
		{Name: "decay", Min: 0, Max: 4},             // multiplier (x)
		{Name: "amp_attack", Min: 0, Max: 2, Unit: "s"}, // seconds
		{Name: "cutoff", Min: 20, Max: 20000, Unit: "Hz"},
	}
	for _, def := range defs {
		def := def
		t.Run(def.Name, func(t *testing.T) {
			badge := NewKnobStepBadge(def)
			// Walk to the finest rung first, then iterate fine -> coarse.
			for badge.AdjustResolution(1) {
			}
			for {
				step := badge.Step()
				w, k := openWheelForPrecision(t, def, 0.1, step)
				before := w.liveValueLabel()
				k.NudgeEndless(knobEndlessPxPerNotch) // exactly one step
				if realValue(k) == def.Min+0.1*(def.Max-def.Min) {
					t.Fatalf("step %v: nudge did not move the value (no headroom)", step)
				}
				after := w.liveValueLabel()
				if before == after {
					t.Fatalf("step %v (unit %q): center value frozen at %q after a one-step nudge",
						step, def.Unit, before)
				}
				if !badge.AdjustResolution(-1) {
					break
				}
			}
		})
	}
}
