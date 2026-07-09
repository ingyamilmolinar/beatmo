package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// TestFreshInitialRowVolumeIs50 verifies a freshly constructed DrumView's
// first (seeded) row starts at 50% volume, giving users headroom to grow
// (see loudness-normalization plan, Task 4).
func TestFreshInitialRowVolumeIs50(t *testing.T) {
	dv := newTestDrumView(t, 1280, 720)
	if got := dv.Rows[0].Volume; got != 0.5 {
		t.Errorf("fresh initial row Volume = %v, want 0.5", got)
	}
}

// TestAddedRowVolumeIs50 verifies a runtime-added row also defaults to 50%.
func TestAddedRowVolumeIs50(t *testing.T) {
	dv := newTestDrumView(t, 1280, 720)
	before := len(dv.Rows)
	dv.AddRow()
	if len(dv.Rows) != before+1 {
		t.Fatalf("AddRow did not append a row")
	}
	if got := dv.Rows[len(dv.Rows)-1].Volume; got != 0.5 {
		t.Errorf("added row Volume = %v, want 0.5", got)
	}
}

// TestFreshMasterVolumeIs50 verifies a fresh master channel (as produced by
// channelManager.reset(), exercised here via the exported audio.Reset())
// starts at 50% volume.
func TestFreshMasterVolumeIs50(t *testing.T) {
	audio.Reset() // rebuilds channels via channelManager.reset()
	if got := audio.MainVolume(); got != 0.5 {
		t.Errorf("fresh master volume = %v, want 0.5", got)
	}
}
