package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Registered and reachable through the tree on mobile, and a tap arms the control inline.
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
	g.drum.Rows[0].Instrument = roster.Snare
	_ = audio.InstallInstrumentDocForTest(roster.Snare, roster.Snare)
	t.Cleanup(func() { audio.ResetInstrumentParams(roster.Snare) })

	g.drum.SetMobileEQMode(true)
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	g.Update()

	knobs := g.drum.SynthTabControls()
	bindings := g.drum.SynthTabBindings()
	wantKnobs := len(audio.InstrumentParamDefs(roster.Snare))
	if len(knobs) != wantKnobs {
		t.Fatalf("mobile: expected %d knobs (wired), got %d", wantKnobs, len(knobs))
	}
	var decayIdx = -1
	for i, b := range bindings {
		if b.def.Name == "amp_decay" {
			decayIdx = i
			break
		}
	}
	if decayIdx < 0 {
		t.Fatal("no decay knob in mobile layout")
	}
	// Decay lives in ENVELOPE.
	selectSectionForKnobIdx(t, g, roster.Snare, decayIdx)
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

	// Armed on RELEASE: OnPress captures so a vertical sweep can scroll instead.
	pressed = true
	g.Update()
	if _, armed := g.drum.synthControlArmed(roster.Snare); armed {
		t.Fatalf("mobile: control armed on the press frame; it must wait for release so a swipe can scroll; rect=%v", rect)
	}
	// The control widget must NOT be in capturing mode (the armed editor owns the value).
	if k.Capturing() {
		t.Fatalf("mobile: control must not be capturing when the armed editor handles the interaction; rect=%v", rect)
	}

	pressed = false
	g.Update()
	if _, armed := g.drum.synthControlArmed(roster.Snare); !armed {
		t.Fatalf("mobile: control not armed after release (tap) at center (%d,%d); rect=%v", cx, cy, rect)
	}
}
