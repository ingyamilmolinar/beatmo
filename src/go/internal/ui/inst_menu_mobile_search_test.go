//go:build test

package ui

import (
	"testing"
)

// TestInstMenuMobileSearchRegistersNativeInput reproduces the reported bug:
// on mobile, the instrument-menu search bar never opens the native keyboard.
// The native keyboard is wired by registering the search field's rect with the
// mobile native-input bridge (mobileInputRegister("inst-search", ...) in
// drumview_layout.go). That registration is gated on dv.instSearchRect being
// non-empty — but dv.instSearchRect is never populated from the menu component,
// so it stays the zero rect and the field is never registered (no keyboard).
func TestInstMenuMobileSearchRegistersNativeInput(t *testing.T) {
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })
	resetTouchOverride()
	t.Cleanup(resetTouchOverride)
	testMobileInputRegistered = map[string]bool{}
	t.Cleanup(func() { testMobileInputRegistered = nil })

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	dv := g.drum

	dv.openInstMenuForRow(0)
	for i := 0; i < 3; i++ {
		_ = g.Update()
	}
	comp := dv.instMenuComp
	if comp == nil || !comp.IsOpen() {
		t.Fatal("instrument menu should be open")
	}
	if comp.Mode() == InstMenuModeCategories {
		cats := comp.CategoryBtns()
		if len(cats) > 0 && cats[0].OnClick != nil {
			cats[0].OnClick()
		}
		for i := 0; i < 3; i++ {
			_ = g.Update()
		}
	}
	if comp.Mode() != InstMenuModeInstruments {
		t.Fatalf("expected instruments mode (search box present), got %v", comp.Mode())
	}
	// Extra frames so the layout-time mobile-input registration runs.
	for i := 0; i < 3; i++ {
		_ = g.Update()
	}

	if !testMobileInputRegistered["inst-search"] {
		t.Fatalf("SEARCH NATIVE-KEYBOARD BUG: instrument-menu search field was not "+
			"registered with the mobile native-input bridge, so tapping it cannot open "+
			"the native keyboard (dv.instSearchRect=%v empty=%v)",
			dv.instSearchRect, dv.instSearchRect.Empty())
	}
}
