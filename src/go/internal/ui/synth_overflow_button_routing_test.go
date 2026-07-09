//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// setupNarrowSynthOverflow builds a mobile synth tab narrow enough that the
// header action buttons collapse into the overflow (⋯) chevron, and returns
// the game plus the overflow button.
func setupNarrowSynthOverflow(t *testing.T, w, h int) (*Game, *Button) {
	t.Helper()
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.SetForceMobileProfile(true)
	g.Layout(w, h)
	uiNode := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = uiNode.ID
	g.drum.Rows[0].Node = uiNode
	g.drum.Rows[0].Instrument = "snare"
	audio.BindInstrumentToRecipe("snare", "drum-snare")
	t.Cleanup(func() { audio.ResetInstrumentParams("snare") })
	g.drum.SetMobileEQMode(true)
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	g.Update()

	var ov *Button
	for _, b := range g.drum.SynthTabButtons() {
		if b != nil && b.Text == "" && b.Icon == string(IconOverflow) {
			ov = b
			break
		}
	}
	return g, ov
}

// TestSynthOverflowButtonOpensSheet reproduces a real bug: on a narrow synth
// panel the header collapses extra actions into the overflow (⋯) button. But
// the footer hit-dispatch loop routes buttons by their sentinel TEXT, and the
// overflow button's empty text falls into the `default` case that is wired to
// the RESET handler. So tapping the overflow chevron wipes the synth params
// back to defaults instead of opening the actions menu.
func TestSynthOverflowButtonOpensSheet(t *testing.T) {
	g, ov := setupNarrowSynthOverflow(t, 360, 760)
	if ov == nil || ov.Rect().Empty() {
		t.Skip("overflow button not present at this size — layout changed")
	}
	r := ov.Rect()
	cx := (r.Min.X + r.Max.X) / 2
	cy := (r.Min.Y + r.Max.Y) / 2

	// Dirty a param so we can detect an accidental Reset.
	audio.SetInstrumentParam("snare", "amp", 0.123)

	clickGame(g, cx, cy)

	portal := g.drum.portal()
	if portal == nil || !portal.Has("synth-overflow-sheet") {
		t.Fatalf("tapping the overflow ⋯ button did not open the overflow sheet "+
			"(portal open=%v) — the footer dispatch mis-routed it to another handler",
			portal != nil && portal.IsOpen())
	}
	if amp := audio.GetInstrumentParams("snare")["amp"]; amp != 0.123 {
		t.Fatalf("tapping the overflow ⋯ button changed amp %v→%v — it fired Reset instead of opening the menu",
			0.123, amp)
	}
}
