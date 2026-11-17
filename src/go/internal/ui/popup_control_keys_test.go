//go:build test

package ui

// popup_control_keys_test.go — the PARTIAL keyboard-claim contract
// (keyboard_key_claim.go): a value control that pops up over a tab owns the
// keys it handles and NOTHING else.
//
// Regression: opening the Sampler tab's Pitch control and pressing an arrow key
// panned the grid camera instead of moving the value — the popup handled no
// arrow keys AND declared no claim, so gridShortcut's arrow-pan ran unopposed.

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// pressKeyThroughShortcuts drives ONE key press edge through the real
// Game.handleGlobalShortcuts path — the same entry point Game.Update uses — so
// claimant dispatch, key suppression and the gated router all run in order.
func pressKeyThroughShortcuts(g *Game, k ebiten.Key) {
	withStubbedKeys(nil, []ebiten.Key{k}, func() { g.handleGlobalShortcuts() })
}

// camOffsets snapshots the grid camera pan so a test can assert it did NOT move.
func camOffsets(g *Game) (float64, float64) { return g.cam.OffsetX, g.cam.OffsetY }

// TestSamplerWheelPopupArrowsEditValueNotCamera is the reported bug: with the
// Sampler Pitch wheel open, Up/Down must move the VALUE and Left/Right the step
// RESOLUTION, and neither may pan the grid.
func TestSamplerWheelPopupArrowsEditValueNotCamera(t *testing.T) {
	g, _ := newMobileSamplerWheelGame(t)
	dv := g.drum
	dv.openSamplerKnobWheelPopup(samplerKnobTranspose)
	if !dv.samplerWheelPopup.IsOpen() {
		t.Fatal("sampler wheel popup did not open")
	}
	k := dv.sampler.knobs[samplerKnobTranspose]
	badge := dv.sampler.knobStepBadges[samplerKnobTranspose]

	x0, y0 := camOffsets(g)

	before := k.Value
	pressKeyThroughShortcuts(g, ebiten.KeyArrowUp)
	if k.Value <= before {
		t.Errorf("ArrowUp must raise the value: %v -> %v", before, k.Value)
	}
	up := k.Value
	pressKeyThroughShortcuts(g, ebiten.KeyArrowDown)
	if k.Value >= up {
		t.Errorf("ArrowDown must lower the value: %v -> %v", up, k.Value)
	}

	if badge != nil {
		coarse := badge.Step()
		pressKeyThroughShortcuts(g, ebiten.KeyArrowLeft) // finer
		if badge.Step() >= coarse {
			t.Errorf("ArrowLeft must move the resolution FINER: %v -> %v", coarse, badge.Step())
		}
		fine := badge.Step()
		pressKeyThroughShortcuts(g, ebiten.KeyArrowRight) // coarser
		if badge.Step() <= fine {
			t.Errorf("ArrowRight must move the resolution COARSER: %v -> %v", fine, badge.Step())
		}
		if k.StepMul != badge.Step() {
			t.Errorf("knob StepMul must track the badge rung: %v vs %v", k.StepMul, badge.Step())
		}
	}

	if x, y := camOffsets(g); x != x0 || y != y0 {
		t.Fatalf("grid panned while the wheel popup was open: (%v,%v) -> (%v,%v)", x0, y0, x, y)
	}
}

// TestSamplerWheelPopupLetsUnclaimedKeysThrough is the other half of the
// contract: the popup owns the arrows ONLY. Zoom / reset / transport keys must
// still reach their own shortcut nodes while it is open.
func TestSamplerWheelPopupLetsUnclaimedKeysThrough(t *testing.T) {
	g, _ := newMobileSamplerWheelGame(t)
	dv := g.drum
	dv.openSamplerKnobWheelPopup(samplerKnobTranspose)

	zoom := g.cam.Scale
	pressKeyThroughShortcuts(g, ebiten.KeyBracketRight)
	if g.cam.Scale == zoom {
		t.Error("] must still zoom while a value popup is open (only the arrows are claimed)")
	}

	// TriggerPlayPause raises a one-frame pulse that Game.Update consumes; the
	// pulse itself is the proof that the transport node saw the key.
	dv.playPressed = false
	pressKeyThroughShortcuts(g, ebiten.KeySpace)
	if !dv.playPressed {
		t.Error("Space must still reach the transport while a value popup is open")
	}
	dv.playPressed = false
}

// TestClosedWheelPopupReleasesTheArrows: once the popup closes, the arrows must
// go back to panning the grid — a claim that outlives its control would silently
// kill arrow-pan for the rest of the session.
func TestClosedWheelPopupReleasesTheArrows(t *testing.T) {
	g, _ := newMobileSamplerWheelGame(t)
	dv := g.drum
	dv.openSamplerKnobWheelPopup(samplerKnobTranspose)
	dv.closeSamplerKnobWheelPopup()
	if dv.samplerWheelPopup.IsOpen() {
		t.Fatal("popup should be closed")
	}
	_, y0 := camOffsets(g)
	pressKeyThroughShortcuts(g, ebiten.KeyArrowUp)
	if _, y := camOffsets(g); y == y0 {
		t.Fatal("arrow-pan must resume once the value popup is closed")
	}
}

// TestEQWheelPopupArrowsEditValueNotCamera pins the same contract for the EQ
// tab's precision wheel — the second MobileWheelPopup instance, reached by a
// different opener, so a fix applied to only one site is caught here.
func TestEQWheelPopupArrowsEditValueNotCamera(t *testing.T) {
	assertDefaultParityState(t)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	dv := g.drum
	dv.eqPanelZone.SetActiveTab(TabEQ)

	dv.openEQKnobWheelPopup(0)
	if !dv.eqWheelPopup.IsOpen() {
		t.Fatal("EQ wheel popup did not open")
	}
	before := dv.eqPanelZone.bandGainsDB[0]
	x0, y0 := camOffsets(g)

	pressKeyThroughShortcuts(g, ebiten.KeyArrowUp)

	if dv.eqPanelZone.bandGainsDB[0] <= before {
		t.Errorf("ArrowUp must raise the EQ band gain: %v -> %v", before, dv.eqPanelZone.bandGainsDB[0])
	}
	if x, y := camOffsets(g); x != x0 || y != y0 {
		t.Fatalf("grid panned while the EQ wheel popup was open: (%v,%v) -> (%v,%v)", x0, y0, x, y)
	}
}

// TestVolumePopupArrowsAdjustVolumeNotCamera pins the contract for the slider
// popups (row volume / master volume), the third popup family.
func TestVolumePopupArrowsAdjustVolumeNotCamera(t *testing.T) {
	assertDefaultParityState(t)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	dv := g.drum
	dv.Bounds = image.Rect(0, 400, 1280, 720)

	dv.openVolumePopup(0)
	if !dv.volPopup.IsOpen() {
		t.Fatal("volume popup did not open")
	}
	before := dv.Rows[0].Volume
	x0, y0 := camOffsets(g)

	pressKeyThroughShortcuts(g, ebiten.KeyArrowDown)

	if dv.Rows[0].Volume >= before {
		t.Errorf("ArrowDown must lower the row volume: %v -> %v", before, dv.Rows[0].Volume)
	}
	if x, y := camOffsets(g); x != x0 || y != y0 {
		t.Fatalf("grid panned while the volume popup was open: (%v,%v) -> (%v,%v)", x0, y0, x, y)
	}
}
