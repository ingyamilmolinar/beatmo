//go:build test

package ui

import (
	"image"
	"strconv"
	"testing"
)

// TestParamEditorOpenValueArmsMobileInput closes the disclosed coverage gap
// from the Task 6 review: param_value_editor.go's open-time
// mobileInputRegister call (OpenValue, ~line 148) — the ONE call the
// discipline test exempts from the tree-owned seam — had no test proving it
// actually fires. This drives the real ParamValueEditor.OpenValue path (the
// same call synth_panel_zone.go's openSynthParamEditor, eq_panel_zone.go, and
// sampler_panel_zone.go make) on a mobile profile and asserts the low-level
// mobile-input stub observed a registration for the given MobileInputID at
// open time — not merely that syncNativeGestures later re-arms it (that half
// is covered separately by TestParamEditorNativeRectArmedBySync).
//
// Non-vacuous: commenting out the `mobileInputRegister(...)` call at
// param_value_editor.go:148 makes this test fail (verified manually during
// authoring — see the task report). The stub's testMobileInputRegistered /
// testMobileInputRect maps are populated ONLY by mobileInputRegister itself
// (mobile_input_stub.go), so there is no other path that could make this
// assertion pass.
func TestParamEditorOpenValueArmsMobileInput(t *testing.T) {
	assertDefaultParityState(t)
	restore := SetRuntimeProfileForTest(browserRuntimeProfile())
	defer restore()
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.SetForceMobileProfile(true)
	g.Layout(390, 720)
	dv := g.drum

	registered := map[string]bool{}
	rects := map[string]image.Rectangle{}
	prevReg, prevRect := testMobileInputRegistered, testMobileInputRect
	testMobileInputRegistered = registered
	testMobileInputRect = rects
	t.Cleanup(func() {
		testMobileInputRegistered = prevReg
		testMobileInputRect = prevRect
	})

	if dv.paramEditor == nil {
		dv.paramEditor = NewParamValueEditor()
	}
	const mobileID = "synth-param"
	dv.paramEditor.OpenValue(ValueOpen{
		Spec: ValueSpec{
			Format: func(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) },
			Parse: func(s string) (float64, bool) {
				v, err := strconv.ParseFloat(s, 64)
				return v, err == nil
			},
			MinW:   72,
		},
		Anchor:        image.Rect(10, 10, 90, 40),
		MobileInputID: mobileID,
		Get:           func() float64 { return 0.5 },
		Set:           func(float64) {},
	})

	if !registered[mobileID] {
		t.Fatalf("OpenValue did not call mobileInputRegister for MobileInputID %q at open time — "+
			"mobile param entry would show a Go text box with no live native <input> underneath", mobileID)
	}
	if got := rects[mobileID]; got.Empty() {
		t.Fatalf("registered rect for %q is empty: %v", mobileID, got)
	}
}
