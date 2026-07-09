//go:build test

package ui

import "testing"

// The desktop boot must showcase the drum rows — the product's core surface.
// Pre-fix, the content-based auto-split capped the drum pane at 50% of the
// window, so the 6-row startup demo booted with roughly ONE visible row
// squeezed between the transport and the audio panel while the grid pane
// kept half the screen (D1 in the 2026-07-04 critique: "an EQ with a graph
// on top"). The cap is now 60%: content that wants the room gets it, and a
// 6-row demo at 720p fits ~4-5 visible rows above the panel.
// (Pure-function pin: the full auto-split path only runs outside go-test,
// and the stub widget board sizes the panel differently — the real-geometry
// verification is the transport_idle screenshot.)
func TestDesktopDrumPaneWantCap(t *testing.T) {
	const h, rowH = 720, 36
	// The -tags test init zeroes eqPanelHeight (drumview_eq_test_override.go);
	// pin the production value so the formula is exercised as shipped.
	prev := eqPanelHeight
	eqPanelHeight = 190
	t.Cleanup(func() { eqPanelHeight = prev })

	// 6 rows want 40+7*36+190 = 482px — more than the cap allows. The cap
	// must yield 60% (432), not the old 50% (360) that buried the rack.
	if got, want := desktopDrumPaneWant(h, 6, rowH), h*3/5; got != want {
		t.Fatalf("6-row demo: drum pane = %d, want the 60%% cap %d", got, want)
	}

	// A single row doesn't want the room — content-based sizing unchanged.
	small := desktopHeaderH + 2*rowH + eqPanelHeight
	if got := desktopDrumPaneWant(h, 1, rowH); got != small {
		t.Fatalf("1-row boot: drum pane = %d, want content size %d", got, small)
	}
}
