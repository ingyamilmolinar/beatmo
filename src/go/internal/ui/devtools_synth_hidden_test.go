//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// The production Synth tab (spec 2026-10-24 §4.2) is category cards with dials
// and the graph — nothing else. No pipeline strip, no "+ Add", no drill into
// raw knobs (a card tap focuses instead), and no card without a dial to turn.

func TestProductionSynthHasNoLowLevelSurfaces(t *testing.T) {
	forceProductionUIForTest(t)
	g := newDialGame(t)
	dv := g.drum
	inst := instrument.ID(dialInst(g))

	if n := len(dv.instEditorPipePills); n != 0 {
		t.Fatalf("production root lays out %d pipeline pills", n)
	}
	if dv.instEditorPipeAddBtn != nil {
		t.Fatal("production root lays out the pipeline \"+ Add\" button")
	}
	if len(dv.instEditorMacroCards) == 0 {
		t.Fatal("production root lays out no category cards")
	}
	for _, c := range dv.instEditorMacroCards {
		if c.nDials == 0 {
			t.Errorf("production root shows card %q with no dial", c.id)
		}
	}

	// A tap on a card's title band (clear of its dials) does not drill.
	c := dv.instEditorMacroCards[0]
	tapAtPoint(g, image.Pt(c.r.Min.X+macroCardInset()+4, c.r.Min.Y+macroCardInset()+4))
	if !dv.synthAtMacroRoot(inst) {
		t.Fatalf("a production card tap drilled into %+v", dv.synthDrillTarget(inst))
	}
	// Every programmatic drill route refuses too.
	if dv.SelectSynthSectionByLabel("FILTER") || dv.SelectSynthSectionByLabel("ENVELOPE") {
		t.Error("SelectSynthSectionByLabel drilled into a stage in the production UI")
	}
	dv.setSynthMacroOpen(inst, c.id)
	dv.setSynthStageOpen(inst, synthSectionFilter)
	if !dv.synthAtMacroRoot(inst) {
		t.Fatalf("a programmatic drill reached %+v in the production UI", dv.synthDrillTarget(inst))
	}
}

func TestDevBuildSynthKeepsLowLevelSurfaces(t *testing.T) {
	if !devToolsBuild {
		t.Skip("release build")
	}
	g := newDialGame(t)
	dv := g.drum
	if len(dv.instEditorPipePills) == 0 {
		t.Fatal("developer build: the pipeline strip has no pills")
	}
	inst := instrument.ID(dialInst(g))
	c := dv.instEditorMacroCards[0]
	tapAtPoint(g, image.Pt(c.r.Min.X+macroCardInset()+4, c.r.Min.Y+macroCardInset()+4))
	if dv.synthAtMacroRoot(inst) {
		t.Fatal("developer build: a card tap must still drill into its knobs")
	}
}

// Hiding dial-less cards and the dials the DSP finds inert must leave every
// instrument something to shape: the floor is 4 dials on 2 cards.
func TestEveryInstrumentKeepsProductionDials(t *testing.T) {
	for _, id := range roster.All {
		schema := audio.InstrumentParamDefs(id)
		if len(schema) == 0 {
			continue
		}
		voice := 0.0
		if d, ok := audio.InstrumentDoc(id); ok {
			voice = d.Render.Seconds
		}
		cards := productionMacroCards(shownRootMacros(id, voice))
		dials := 0
		for _, c := range cards {
			if len(c.Dials) == 0 {
				t.Errorf("%s: productionMacroCards kept dial-less card %q", id, c.ID)
			}
			dials += len(c.Dials)
		}
		if dials < 4 || len(cards) < 2 {
			t.Errorf("%s: production keeps %d dials on %d cards, want >= 4 on >= 2", id, dials, len(cards))
		}
	}
}

// Every instrument's production cards fit without scrolling at 1280x720.
func TestProductionCardsFitTwoRows(t *testing.T) {
	forceProductionUIForTest(t)
	g := newDialGame(t)
	g.Layout(1280, 720)
	dv := g.drum
	for _, id := range roster.All {
		if len(audio.InstrumentParamDefs(id)) == 0 {
			continue
		}
		dv.Rows[0].Instrument = id
		dv.eqPanelZone.SetActiveChannel(channel.Of(id))
		dv.eqPanelZone.Layout(dv.eqPanelZone.PanelRect())
		idleFrame(g)
		grid := dv.instEditorMacroGrid
		if grid == nil {
			t.Fatalf("%s: no card grid", id)
		}
		if grid.HasScroll() {
			t.Errorf("%s: production cards scroll at 1280x720 (%d cards, row %dx%d, area %v)",
				id, len(dv.instEditorMacros), grid.CellW(), grid.CellH(), grid.ContentRect().Size())
		}
	}
}
