//go:build test

package ui

import (
	"image"
	"testing"
)

// TestRowMuteSoloActiveStyleSwap verifies that toggling Muted/Solo on a
// DrumRow flips the per-row mute/solo button style to the *Active variants
// (filled, not just bordered). The plan upgrades these from border-only
// hints to a full fill so users can see the row state at a glance.
func TestRowMuteSoloActiveStyleSwap(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })

	rows := makeTestRows(2)
	z, _ := newTestRowRackZone(rows)
	z.Layout(image.Rect(0, 0, 390, 600))

	if len(z.entries) < 2 {
		t.Fatalf("expected 2 row entries, got %d", len(z.entries))
	}
	mute := z.entries[0].muteBtn
	solo := z.entries[1].soloBtn

	// Drive the toggle visual sync directly (the same helper the draw paths use).
	syncToggleVisual(mute, true, MuteActiveStyle, InstButtonStyle, IconMute, IconMute, nil, nil)
	if mute.Style != MuteActiveStyle {
		t.Errorf("muted row: style = %T, want MuteActiveStyle", mute.Style)
	}

	syncToggleVisual(solo, true, SoloActiveStyle, InstButtonStyle, IconSolo, IconSolo, nil, nil)
	if solo.Style != SoloActiveStyle {
		t.Errorf("soloed row: style = %T, want SoloActiveStyle", solo.Style)
	}

	// Inactive paths must revert to the default.
	syncToggleVisual(mute, false, MuteActiveStyle, InstButtonStyle, IconMute, IconMute, nil, nil)
	if mute.Style != InstButtonStyle {
		t.Errorf("un-muted row: style should revert to InstButtonStyle, got %T", mute.Style)
	}
	syncToggleVisual(solo, false, SoloActiveStyle, InstButtonStyle, IconSolo, IconSolo, nil, nil)
	if solo.Style != InstButtonStyle {
		t.Errorf("un-soloed row: style should revert to InstButtonStyle, got %T", solo.Style)
	}

	// MuteActiveStyle must use the destructive-mute fill (colMuteRed); SoloActive
	// must use the bright accent. Bare structural assertions — pixel verification
	// of the fill colors lives in the regenerated visual baseline.
	if MuteActiveStyle.Fill == InstButtonStyle.Fill {
		t.Error("MuteActiveStyle and InstButtonStyle must not share a fill — active state must visibly differ")
	}
	if SoloActiveStyle.Fill == InstButtonStyle.Fill {
		t.Error("SoloActiveStyle and InstButtonStyle must not share a fill — active state must visibly differ")
	}
}
