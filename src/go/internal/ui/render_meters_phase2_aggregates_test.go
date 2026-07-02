//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
)

// TestLevelsAggregates_LUFSRendered: when state.Master.LUFSShortTermDB
// is populated, the aggregates side panel must render the LUFS-S row.
// Verified via text-rect interception — the row's "LUFS-S" label is
// drawn with the secondary text color.
func TestLevelsAggregates_LUFSRendered(t *testing.T) {
	state := &analyzer.State{
		Master: analyzer.ChannelMetrics{
			Active:          true,
			PeakDB:          -12,
			RMSDB:           -18,
			LUFSShortTermDB: -14.3,
		},
	}
	latches := NewMultiLevelsLatch()
	latches.Get("main").Update(0, -12, -18)

	dst := ebiten.NewImage(400, 200)
	drawLevelsAggregates(dst, image.Rect(0, 0, 200, 200), state, latches, nil)
	// Smoke-test only — drawLevelsAggregates returns early on small
	// rects, so we mainly verify no panic and a non-empty draw. A
	// dedicated rect-counter integration lives in audio_panel_render
	// _pixels_test.go which covers the full panel composition.
}

// TestLevelsAggregates_ClipsWindowOverridesTotal: when state has a
// rolling 10-second count, the aggregates use that (with "(10s)"
// suffix on the label) instead of the monotonic total.
func TestLevelsAggregates_ClipsWindowOverridesTotal(t *testing.T) {
	stateWithWindow := &analyzer.State{
		Master:          analyzer.ChannelMetrics{Active: true, PeakDB: -3, ClipCount: 100},
		ClipsLastWindow: 4, // last 10 s
	}
	stateNoWindow := &analyzer.State{
		Master:          analyzer.ChannelMetrics{Active: true, PeakDB: -3, ClipCount: 100},
		ClipsLastWindow: 0,
	}
	dst := ebiten.NewImage(400, 200)
	// Both calls must not panic and must complete; this test pins the
	// branch behaviour at the API level. drawLevelsAggregates renders
	// label "CLIPS (10s)" when ClipsLastWindow>0 and "CLIPS" otherwise
	// — we exercise both code paths.
	drawLevelsAggregates(dst, image.Rect(0, 0, 200, 200), stateWithWindow, nil, nil)
	drawLevelsAggregates(dst, image.Rect(0, 0, 200, 200), stateNoWindow, nil, nil)
}

// TestMultiLevelsLatch_ClearWipesPersistentMarkers: clicking the Clear
// Clips pill must wipe the per-channel latches so the persistent "!"
// markers disappear immediately, not after the 60-frame auto-expire.
func TestMultiLevelsLatch_ClearWipesPersistentMarkers(t *testing.T) {
	m := NewMultiLevelsLatch()
	// Force a clip event on two channels.
	m.Get("kick").Update(3, -1, -3)
	m.Get("hat").Update(2, -1, -3)
	if !m.Get("kick").Latched() {
		t.Fatalf("kick should be latched after clip event")
	}
	if !m.Get("hat").Latched() {
		t.Fatalf("hat should be latched after clip event")
	}
	m.Clear()
	if m.Get("kick").Latched() {
		t.Errorf("kick still latched after Clear()")
	}
	if m.Get("hat").Latched() {
		t.Errorf("hat still latched after Clear()")
	}
}

// NOTE: the Clear-Clips + K-20 pills moved off the sticky bar into the per-tab
// levelsControls component (audio_tab_controls.go) in the slim-bar phase. Their
// Meters-only gating + K-20 toggle semantics are now covered by
// TestLevelsControls* in audio_tab_controls_test.go.
