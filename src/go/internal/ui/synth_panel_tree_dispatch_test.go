package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestSynthTab_ControlTapViaTreeDispatch_Mobile verifies that on mobile the synth
// knob hit-area is correctly registered (non-empty rect, reachable via the
// tree dispatcher) and that a tap opens the scroll-wheel popup (the mobile
// knob-interaction path introduced by the mobile-wheel feature). The old drag
// path is replaced by the wheel on mobile; desktop drag is tested separately.
func TestSynthTab_ControlTapViaTreeDispatch_Mobile(t *testing.T) {
	assertDefaultParityState(t)
	restoreProfile := SetRuntimeProfileForTest(browserRuntimeProfile())
	t.Cleanup(restoreProfile)

	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.SetForceMobileProfile(true)
	g.Layout(414, 896)
	uiNode := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = uiNode.ID
	g.drum.Rows[0].Node = uiNode
	g.drum.Rows[0].Name = "Snare"
	g.drum.Rows[0].Instrument = "snare"
	audio.BindInstrumentToRecipe("snare", "drum-snare")
	t.Cleanup(func() { audio.ResetInstrumentParams("snare") })

	g.drum.SetMobileEQMode(true)
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	g.Update()

	knobs := g.drum.SynthTabControls()
	bindings := g.drum.SynthTabBindings()
	wantKnobs := len(audio.WiredParamsForRecipe("drum-snare"))
	if len(knobs) != wantKnobs {
		t.Fatalf("mobile: expected %d knobs (wired), got %d", wantKnobs, len(knobs))
	}
	var decayIdx = -1
	for i, b := range bindings {
		if b.def.Name == "decay" {
			decayIdx = i
			break
		}
	}
	if decayIdx < 0 {
		t.Fatal("no decay knob in mobile layout")
	}
	// Decay lives in ENVELOPE; open that stage so the knob gets a non-empty,
	// hit-testable rect under the chip-strip + expand-one-detail-pane layout.
	selectSectionForKnobIdx(t, g, "snare", decayIdx)
	g.Update()
	k := g.drum.SynthTabControls()[decayIdx]
	rect := k.Rect()
	if rect.Empty() {
		t.Fatalf("mobile decay knob has empty rect; section layout = %v", g.drum.SynthTabSections())
	}

	cx := (rect.Min.X + rect.Max.X) / 2
	cy := (rect.Min.Y + rect.Max.Y) / 2

	mouseX, mouseY := cx, cy
	pressed := false
	restoreInput := SetInputForTest(
		func() (int, int) { return mouseX, mouseY },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 414, 896 },
	)
	defer restoreInput()

	// On mobile, a tap on the control cell opens the scroll-wheel popup — but on
	// RELEASE, not on the down frame. synthControlHitAdapter.OnPress captures the
	// gesture (InputCaptured) so a vertical sweep can scroll instead of opening the
	// popup; a stationary tap opens it when the finger lifts.
	pressed = true
	g.Update()
	if g.drum.synthWheelPopup.IsOpen() {
		t.Fatalf("mobile: wheel popup opened on the press frame; it must wait for release so a swipe can scroll; rect=%v", rect)
	}
	// The control widget must NOT be in capturing mode (the wheel owns the value).
	if k.Capturing() {
		t.Fatalf("mobile: control must not be capturing when wheel popup handles the interaction; rect=%v", rect)
	}

	pressed = false
	g.Update()
	if !g.drum.synthWheelPopup.IsOpen() {
		t.Fatalf("mobile: wheel popup not open after release (tap) at center (%d,%d); rect=%v", cx, cy, rect)
	}
}
