//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Narrow enough that the header actions collapse into ⋯.
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
	g.drum.Rows[0].Instrument = roster.Snare
	_ = audio.InstallInstrumentDocForTest(roster.Snare, roster.Snare)
	t.Cleanup(func() { audio.ResetInstrumentParams(roster.Snare) })
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

// Buttons are routed by sentinel text, and the overflow's empty text fell to the default case wired to Reset: ⋯
// must open the menu, never reset the params.
func TestSynthOverflowButtonOpensSheet(t *testing.T) {
	g, ov := setupNarrowSynthOverflow(t, 360, 760)
	if ov == nil || ov.Rect().Empty() {
		t.Skip("overflow button not present at this size — layout changed")
	}
	r := ov.Rect()
	cx := (r.Min.X + r.Max.X) / 2
	cy := (r.Min.Y + r.Max.Y) / 2

	// Dirty a param so we can detect an accidental Reset.
	audio.SetInstrumentParam(roster.Snare, "amp_decay", 0.123)

	clickGame(g, cx, cy)

	portal := g.drum.portal()
	if portal == nil || !portal.Has("synth-action-menu") {
		t.Fatalf("tapping the overflow ⋯ button did not open the overflow sheet "+
			"(portal open=%v) — the footer dispatch mis-routed it to another handler",
			portal != nil && portal.IsOpen())
	}
	if amp := audio.GetInstrumentParams(roster.Snare)["amp_decay"]; amp != 0.123 {
		t.Fatalf("tapping the overflow ⋯ button changed amp_decay %v→%v — it fired Reset instead of opening the menu",
			0.123, amp)
	}
}
