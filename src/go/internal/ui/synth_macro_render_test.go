package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// newKickMacroGame builds a single-row game on the shipped modular 808-kick
// instrument (schemaUsesKickStage) with the beginner macro layer on. Uses the
// real "kick-808" id — already bound to synth-modular-kick-808 — so it rebinds
// nothing and can't leak a binding into sibling tests.
func newKickMacroGame(t *testing.T) *Game {
	t.Helper()
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	ui := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = ui.ID
	g.drum.Rows[0].Node = ui
	g.drum.Rows[0].Name = "Kick"
	g.drum.Rows[0].Instrument = roster.Kick808
	t.Cleanup(func() { audio.ResetInstrumentParams(roster.Kick808) })
	return g
}

// A kick row now activates the macro layer with the PERCUSSION concept set,
// instead of falling through to the legacy stage UI.
func TestSynthMacro_KickPopulatesMacroLayer(t *testing.T) {
	g := newKickMacroGame(t)
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.SetActiveChannel(channel.Of(roster.Kick808))
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())

	if g.drum.instEditorMacros == nil {
		t.Fatal("kick row should activate the macro layer, got nil instEditorMacros")
	}
	ids := map[string]bool{}
	for _, m := range g.drum.instEditorMacros {
		ids[m.ID] = true
	}
	if !ids["punch"] || !ids["boom"] {
		t.Errorf("kick macro root should include punch+boom, got %v", ids)
	}
}

// newViolinMacroGame builds a single-row game on the shipped melodic "violin"
// instrument with the macro layer on and a TALL panel (headroom for a grown
// armed card, mirroring growSynthPanelForTest).
func newViolinMacroGame(t *testing.T) *Game {
	t.Helper()
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 2000)
	ui := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = ui.ID
	g.drum.Rows[0].Node = ui
	g.drum.Rows[0].Name = "Violin"
	g.drum.Rows[0].Instrument = roster.Violin
	t.Cleanup(func() { audio.ResetInstrumentParams(roster.Violin) })
	return g
}

func drillMacro(t *testing.T, g *Game, concept string) (inst string, kIdx int) {
	t.Helper()
	dv := g.drum
	dv.eqPanelZone.SetActiveTab(TabSynth)
	dv.eqPanelZone.SetActiveChannel(channel.Of(dv.Rows[0].Instrument))
	dv.eqPanelZone.Layout(dv.eqPanelZone.PanelRect())
	inst = string(dv.resolveSynthInstrument(dv.synthTabActiveInstrument()))
	dv.setSynthMacroOpen(instrument.ID(inst), concept)
	dv.eqPanelZone.Layout(dv.eqPanelZone.PanelRect())
	idxs := dv.macroConceptKnobIdxs(concept)
	if len(idxs) == 0 {
		t.Fatalf("concept %q drilled into no bound knobs", concept)
	}
	return inst, idxs[0]
}

// Arming a drilled control must grow its card in place (to host the two barrel
// lanes), exactly like the stage detail pane. Today the macro path early-returns
// before growArmedCard, so the card never grows and the click looks dead.
func TestSynthMacro_DrilledControlArmsAndGrows(t *testing.T) {
	g := newViolinMacroGame(t)
	dv := g.drum
	inst, kIdx := drillMacro(t, g, "brightness")
	restH := dv.instEditorControls[kIdx].Rect().Dy()
	if restH <= 0 {
		t.Fatalf("resting drilled control rect empty: %v", dv.instEditorControls[kIdx].Rect())
	}
	dv.armSynthControl(instrument.ID(inst), kIdx)
	settleArmGrow(dv)
	armedH := dv.instEditorControls[kIdx].Rect().Dy()
	if armedH <= restH {
		t.Fatalf("armed drilled control should grow taller: rest=%d armed=%d", restH, armedH)
	}
}

// settleArmGrow advances the grow/shrink animation to its settled state, relaying
// out the panel each frame (mirrors how synthTabUpdate drives it in the app).
func settleArmGrow(dv *DrumView) {
	for i := 0; i < 50 && dv.advanceArmGrow(); i++ {
		dv.eqPanelZone.Layout(dv.eqPanelZone.PanelRect())
	}
	dv.eqPanelZone.Layout(dv.eqPanelZone.PanelRect())
}

// Drawing the drilled+armed macro view must lay out the barrel lanes (via
// drawArmedControlCard → SetCardRect), or the control shows no editable barrels.
func TestSynthMacro_DrilledArmedDrawsBarrels(t *testing.T) {
	g := newViolinMacroGame(t)
	dv := g.drum
	inst, kIdx := drillMacro(t, g, "brightness")
	dv.armSynthControl(instrument.ID(inst), kIdx)
	settleArmGrow(dv)
	img := ebiten.NewImage(1280, 2000)
	dv.eqPanelZone.Draw(img)
	if dv.instEditorArmedEditor == nil {
		t.Fatal("armed editor should exist after arming a drilled control")
	}
	if dv.instEditorArmedEditor.ValueLaneRect().Empty() {
		t.Fatal("armed editor value lane empty — barrels not laid out/drawn in the macro drilled view")
	}
}

// End-to-end "click": a stationary tap on a drilled control (via the SAME hit
// adapter production publishes, keyed by synthTabActiveInstrument) must arm it
// AND the next layout must grow its card. This is the exact path the reported
// "clicking the control does nothing" bug lived on — it guards the instID key
// used by the adapter matching the one layout/draw consult.
func TestSynthMacro_DrilledControlClickArmsAndGrows(t *testing.T) {
	g := newViolinMacroGame(t)
	dv := g.drum
	_, kIdx := drillMacro(t, g, "brightness")
	r := dv.instEditorControls[kIdx].Rect()
	if r.Empty() {
		t.Fatal("drilled control has no rect to click")
	}
	restH := r.Dy()
	prodInst := dv.synthTabActiveInstrument()
	if _, armed := dv.synthControlArmed(prodInst); armed {
		t.Fatal("control should not be armed before the click")
	}
	h := &synthControlHitAdapter{dv: dv, idx: kIdx, instID: prodInst}
	h.OnPress(r.Min.X+2, r.Min.Y+2)
	h.OnRelease(r.Min.X+2, r.Min.Y+2) // stationary tap → arm
	if _, armed := dv.synthControlArmed(prodInst); !armed {
		t.Fatal("a stationary tap must ARM the drilled control")
	}
	settleArmGrow(dv)
	if armedH := dv.instEditorControls[kIdx].Rect().Dy(); armedH <= restH {
		t.Fatalf("clicking a drilled control must arm+grow it: rest=%d armed=%d (instID key mismatch?)", restH, armedH)
	}
}
