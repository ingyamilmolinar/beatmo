//go:build test

package ui

import "testing"

// The drum rows are the core surface: content that wants the room gets up to 60% of the window, so a 6-row
// circuit at 720p shows 4-5 rows above the panel. A pure-function pin: the full auto-split only runs outside go
// test.
func TestDesktopDrumPaneWantCap(t *testing.T) {
	const h, rowH = 720, 36
	// The -tags test init zeroes eqPanelHeight.
	prev := eqPanelHeight
	eqPanelHeight = 190
	t.Cleanup(func() { eqPanelHeight = prev })

	// 6 rows want 482 px, past the cap, which must give 60% (432).
	if got, want := desktopDrumPaneWant(h, 6, rowH), h*3/5; got != want {
		t.Fatalf("6-row demo: drum pane = %d, want the 60%% cap %d", got, want)
	}

	// A single row doesn't want the room — content-based sizing unchanged.
	small := desktopHeaderH + 2*rowH + eqPanelHeight
	if got := desktopDrumPaneWant(h, 1, rowH); got != small {
		t.Fatalf("1-row boot: drum pane = %d, want content size %d", got, small)
	}
}
