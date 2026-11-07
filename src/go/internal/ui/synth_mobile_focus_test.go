package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

func newMobileSynthTabGameForTest(t *testing.T) *Game {
	t.Helper()
	return newMobileSynthTabGameForTestSize(t, 390, 720)
}

func newMobileSynthTabGameForTestSize(t *testing.T, w, h int) *Game {
	t.Helper()
	restoreProfile := SetRuntimeProfileForTest(browserRuntimeProfile())
	t.Cleanup(restoreProfile)
	if !forceAutoSize {
		forceAutoSize = true
		t.Cleanup(func() { forceAutoSize = false })
	}

	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	// Nothing else resets this global here, and on the real build a leak fails every later test.
	g.SetForceMobileProfile(true)
	t.Cleanup(func() { g.SetForceMobileProfile(false) })
	g.Layout(w, h)

	uiNode := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = uiNode.ID
	g.drum.Rows[0].Node = uiNode
	g.drum.Rows[0].Name = "Modular"
	g.drum.Rows[0].Instrument = roster.Modular
	_ = audio.InstallInstrumentDocForTest(roster.Modular, roster.Modular)
	t.Cleanup(func() { audio.ResetInstrumentParams(roster.Modular) })

	g.drum.SetMobileEQMode(true)
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.Layout(w, h)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	g.Update() // flush layout/hit index via tree
	return g
}

func drillFirstStageMobile(t *testing.T, g *Game) string {
	t.Helper()
	dv := g.drum
	inst := dv.resolveSynthInstrument(dv.synthTabActiveInstrument())
	if inst == "" || len(dv.instEditorSections) == 0 {
		t.Skip("no synth instrument / stages")
	}
	dv.setSynthStageOpen(inst, dv.instEditorSections[0].id)
	dv.eqPanelZone.Layout(dv.eqPanelZone.PanelRect())
	g.Update()
	return string(inst)
}

// The strip and single-column cards, with no graph band.
func TestMobileSynthTab_RootShowsStripAndCards(t *testing.T) {
	g := newMobileSynthTabGameForTest(t)
	dv := g.drum
	if !dv.synthMobileFocusRect().Empty() {
		t.Fatalf("root reserved a graph band %v", dv.synthMobileFocusRect())
	}
	if len(dv.instEditorPipePills) == 0 {
		t.Fatal("root laid out no pipeline pills on mobile")
	}
	// Every pill fits, the trailing "+" included.
	if _, off := dv.synp().activePipelineStages(dv.synthTabActiveInstrument()); len(off) > 0 {
		if a := dv.instEditorPipeAddBtn; a == nil || a.Rect().Empty() || !a.Rect().In(dv.instEditorPipeRect) {
			t.Fatalf("+ pill missing or outside the strip band on mobile (band %v)", dv.instEditorPipeRect)
		}
	}
	for _, pl := range dv.instEditorPipePills {
		if !pl.rect.In(dv.instEditorPipeRect) {
			t.Fatalf("pill %s %v escapes the strip band %v", sectionLabel(pl.id), pl.rect, dv.instEditorPipeRect)
		}
	}
	if len(dv.instEditorMacroCards) == 0 {
		t.Fatal("root laid out no category cards on mobile")
	}
	if c := dv.instEditorMacroGrid.Cols(); c != 1 {
		t.Fatalf("mobile root card grid Cols()=%d, want a single column", c)
	}
}

// The graph band above the controls, the first card at its ideal height in a 2..4 column grid.
func TestMobileSynthTab_DrilledGraphAboveControls(t *testing.T) {
	g := newMobileSynthTabGameForTestSize(t, 390, 844)
	dv := g.drum
	drillFirstStageMobile(t, g)
	band := dv.synthMobileFocusRect()
	if band.Empty() {
		t.Fatal("drilled view reserved no graph band on mobile")
	}
	d := Profile().DensityValues()
	if band.Dy() < d.SynthRightCardMinH {
		t.Fatalf("graph band %v shorter than SynthRightCardMinH %d", band, d.SynthRightCardMinH)
	}
	order := dv.instEditorDrilledOrder
	if len(order) == 0 {
		t.Fatal("drilled stage has no controls")
	}
	k := dv.instEditorControls[order[0]]
	if k.Rect().Empty() {
		t.Fatal("first ParamCard not laid out")
	}
	if k.Rect().Min.Y < band.Max.Y {
		t.Fatalf("first card %v is not below the graph band %v", k.Rect(), band)
	}
	if got := k.Rect().Dy(); got != d.ParamCardH {
		t.Fatalf("ParamCard height %d != ParamCardH %d", got, d.ParamCardH)
	}
	if c := dv.instEditorMacroGrid.Cols(); c < 2 || c > 4 {
		t.Errorf("mobile drilled grid Cols()=%d, want 2..4", c)
	}
}
