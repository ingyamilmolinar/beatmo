//go:build test

package ui

import (
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestHomeRowIconsAreMutedGreyNotWhite guards the desktop home-row (transport)
// recolor: the play/stop icons — previously the near-white on-surface tint —
// read as the calmer on-surface-muted grey (colTextSecondary), so the home row
// reads like the instrument-row labels rather than glaring white. The record
// icon keeps its semantic red. Uses the shared theme tokens (no new colors).
func TestHomeRowIconsAreMutedGreyNotWhite(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	advanceFrames(g, 2)
	z := g.drum.transportZone

	if got := z.playBtn.IconColor; !rgbaEq(got, colTextSecondary) {
		t.Errorf("play icon color = %v, want muted grey colTextSecondary %v", got, colTextSecondary)
	}
	if got := z.stopBtn.IconColor; !rgbaEq(got, colTextSecondary) {
		t.Errorf("stop icon color = %v, want muted grey colTextSecondary %v", got, colTextSecondary)
	}
	// The bright near-white on-surface must no longer be used for these icons.
	if rgbaEq(z.playBtn.IconColor, colTextPrimary) {
		t.Errorf("play icon still uses near-white colTextPrimary")
	}
	// Record stays semantically red (idle tint), never greyed.
	if rgbaEq(z.recordBtn.IconColor, colTextSecondary) {
		t.Errorf("record icon should stay red, not muted grey")
	}
}
