package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// fxActiveStyle / fxInactiveStyle are the FX-chip styles the row rack
// applies: FX-on = the full instrument hue, FX-off = the neutral row-control
// body (see row_instrument_shades.go).
func fxActiveStyle(dv *DrumView, row int) rowToggleVisual {
	return rowToggleStyle(dv.Rows[row].Color, roleFX, true)
}
func fxInactiveStyle(dv *DrumView, row int) rowToggleVisual {
	return rowToggleStyle(dv.Rows[row].Color, roleFX, false)
}

// fxButtonForRow returns the FX button for the given row.
func fxButtonForRow(t *testing.T, dv *DrumView, row int) *Button {
	t.Helper()
	btns := dv.rowFXBtns()
	if row >= len(btns) {
		t.Fatalf("no FX button for row %d (have %d)", row, len(btns))
	}
	return btns[row]
}

// renderRowControlsDirect forces a fresh draw of the row control rack so the
// FX button's Style reflects current state.
func renderRowControlsDirect(t *testing.T, dv *DrumView) {
	t.Helper()
	dst := ebiten.NewImage(800, 300)
	dv.rowRackZone.drawRowControlsDirect(dst)
}

func TestFXButton_HighlightedWhenAnyEffectEnabled(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 || len(dv.rowFXBtns()) == 0 {
		t.Skip("no rows or FX buttons")
	}
	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)

	renderRowControlsDirect(t, dv)
	btn := fxButtonForRow(t, dv, 0)
	if btn.Style != fxActiveStyle(dv, 0) {
		t.Errorf("with one enabled effect, expected the active instrument-hue FX style, got %v", btn.Style)
	}
}

func TestFXButton_InactiveWhenAllEffectsDisabled(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 || len(dv.rowFXBtns()) == 0 {
		t.Skip("no rows or FX buttons")
	}
	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)

	renderRowControlsDirect(t, dv)
	btn := fxButtonForRow(t, dv, 0)
	if btn.Style != fxActiveStyle(dv, 0) {
		t.Fatalf("precondition: expected active instrument-hue FX style with one enabled effect, got %v", btn.Style)
	}

	// Disable the only effect — button must become inactive.
	audio.ToggleInsertEffect(instID, 0, false)
	renderRowControlsDirect(t, dv)
	if btn.Style != fxInactiveStyle(dv, 0) {
		t.Errorf("with all effects disabled, expected dim instrument-hue FX style, got %v", btn.Style)
	}

	// Re-enable — button must become highlighted again.
	audio.ToggleInsertEffect(instID, 0, true)
	renderRowControlsDirect(t, dv)
	if btn.Style != fxActiveStyle(dv, 0) {
		t.Errorf("after re-enable, expected active instrument-hue FX style, got %v", btn.Style)
	}
}

func TestFXButton_StaysHighlightedIfAnyEffectEnabled(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 || len(dv.rowFXBtns()) == 0 {
		t.Skip("no rows or FX buttons")
	}
	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
	audio.AddInsertEffect(instID, audio.EffectDelay, nil)

	btn := fxButtonForRow(t, dv, 0)

	renderRowControlsDirect(t, dv)
	if btn.Style != fxActiveStyle(dv, 0) {
		t.Fatalf("both enabled, expected active instrument-hue FX style, got %v", btn.Style)
	}

	// Disable just one — still highlighted because second remains enabled.
	audio.ToggleInsertEffect(instID, 0, false)
	renderRowControlsDirect(t, dv)
	if btn.Style != fxActiveStyle(dv, 0) {
		t.Errorf("one of two enabled, expected active instrument-hue FX style, got %v", btn.Style)
	}

	// Disable the second — now inactive.
	audio.ToggleInsertEffect(instID, 1, false)
	renderRowControlsDirect(t, dv)
	if btn.Style != fxInactiveStyle(dv, 0) {
		t.Errorf("both disabled, expected dim instrument-hue FX style, got %v", btn.Style)
	}
}

func TestFXButton_InactiveAfterRemovingAllEffects(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 || len(dv.rowFXBtns()) == 0 {
		t.Skip("no rows or FX buttons")
	}
	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)

	btn := fxButtonForRow(t, dv, 0)
	renderRowControlsDirect(t, dv)
	if btn.Style != fxActiveStyle(dv, 0) {
		t.Fatalf("precondition: expected active instrument-hue FX style, got %v", btn.Style)
	}

	audio.RemoveInsertEffect(instID, 0)
	renderRowControlsDirect(t, dv)
	if btn.Style != fxInactiveStyle(dv, 0) {
		t.Errorf("after removing all effects, expected dim instrument-hue FX style, got %v", btn.Style)
	}
}

// Cache-invalidation regression: when FX active state flips, the controls
// cache must report invalid so the next Draw() rebuilds with the correct
// style. Without this, a Mute/Solo-only validity check silently shows a stale
// highlighted button after the user disables or removes all effects.

func TestFXButton_ControlsCacheInvalidatesOnDisable(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 || len(dv.rowFXBtns()) == 0 {
		t.Skip("no rows or FX buttons")
	}
	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)

	dst := ebiten.NewImage(800, 300)
	dv.rowRackZone.drawRowControls(dst)
	if !dv.rowRackZone.controlsCacheValid() {
		t.Fatal("precondition: cache should be valid right after a populated draw")
	}

	audio.ToggleInsertEffect(instID, 0, false)
	if dv.rowRackZone.controlsCacheValid() {
		t.Error("cache reported valid after disabling all effects; UI would show stale FX highlight")
	}
}

func TestFXButton_ControlsCacheInvalidatesOnRemove(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 || len(dv.rowFXBtns()) == 0 {
		t.Skip("no rows or FX buttons")
	}
	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)

	dst := ebiten.NewImage(800, 300)
	dv.rowRackZone.drawRowControls(dst)
	if !dv.rowRackZone.controlsCacheValid() {
		t.Fatal("precondition: cache should be valid right after a populated draw")
	}

	audio.RemoveInsertEffect(instID, 0)
	if dv.rowRackZone.controlsCacheValid() {
		t.Error("cache reported valid after removing all effects; UI would show stale FX highlight")
	}
}

func TestFXButton_ControlsCacheInvalidatesOnEnable(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 || len(dv.rowFXBtns()) == 0 {
		t.Skip("no rows or FX buttons")
	}
	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
	audio.ToggleInsertEffect(instID, 0, false)

	dst := ebiten.NewImage(800, 300)
	dv.rowRackZone.drawRowControls(dst)
	if !dv.rowRackZone.controlsCacheValid() {
		t.Fatal("precondition: cache should be valid right after a populated draw")
	}

	audio.ToggleInsertEffect(instID, 0, true)
	if dv.rowRackZone.controlsCacheValid() {
		t.Error("cache reported valid after enabling an effect; UI would show stale inactive FX button")
	}
}
