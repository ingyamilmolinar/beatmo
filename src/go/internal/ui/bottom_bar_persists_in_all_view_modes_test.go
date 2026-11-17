//go:build test

package ui

import (
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestBottomBar_PersistsInAllViewModes (Theme 1) verifies that the
// 6-segment bottom-bar view switcher remains visible inside the bottom
// action bar in EVERY mobile view mode (Pads/EQ/Wave/Spec/Mtr/Scope).
// The previous design dimmed/hid the bar when the EQ panel expanded,
// stranding the user inside the panel.
func TestBottomBar_PersistsInAllViewModes(t *testing.T) {
	assertDefaultParityState(t)
	setupMobileTest(t, true)

	for _, mode := range []viewMode{
		viewModeRows,
		viewModeEQ,
		viewModeWave,
		viewModeSpectrum,
		viewModeMeters,
		viewModeChain,
	} {
		logger := game_log.New(testLogOutput(), game_log.LevelError)
		g := New(logger)
		g.Layout(360, 700)
		advanceFrames(g, 2)
		dv := g.drum
		dv.setViewMode(mode)
		advanceFrames(g, 2)

		sc := dv.viewSwitchSegmented
		if sc == nil || sc.Rect().Empty() {
			t.Errorf("mode=%v: segmented control rect empty", mode)
			g.CloseForTest()
			continue
		}
		if !sc.Rect().Overlaps(dv.bottomActionBarRect) {
			t.Errorf("mode=%v: segmented %v doesn't overlap bar %v", mode, sc.Rect(), dv.bottomActionBarRect)
		}
		for i := 0; i < 6; i++ {
			if sc.SegmentRect(i).Empty() {
				t.Errorf("mode=%v: segment %d empty", mode, i)
			}
		}
		g.CloseForTest()
	}
}
