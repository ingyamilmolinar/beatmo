//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// layoutSynthFor drives the Synth tab through the real zone layout for inst.
func layoutSynthFor(t *testing.T, g *Game, inst channel.ID) {
	t.Helper()
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.SetActiveChannel(inst)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
}

func pipePillFor(g *Game, id synthSectionID) (synthPipePill, bool) {
	for _, p := range g.drum.instEditorPipePills {
		if p.id == id {
			return p, true
		}
	}
	return synthPipePill{}, false
}

// The strip shows only ON stages, in pipeline order, and grows a "+" pill
// while any stage is off.
func TestPipelineStrip_SkipsOffStagesKeepsOrder(t *testing.T) {
	g := newViolinMacroGame(t)
	layoutSynthFor(t, g, channel.Of(roster.Violin))
	audio.SetInstrumentParam(roster.Violin, "lfo_enabled", 0)
	audio.SetInstrumentParam(roster.Violin, "burst_enabled", 0)
	layoutSynthFor(t, g, channel.Of(roster.Violin))
	if len(g.drum.instEditorPipePills) == 0 {
		t.Fatal("no pipeline pills laid out")
	}
	for _, pill := range g.drum.instEditorPipePills {
		if pill.id == synthSectionLFO || pill.id == synthSectionBurst {
			t.Fatalf("off stage %s shown in strip", sectionLabel(pill.id))
		}
		if pill.rect.Empty() || pill.btn == nil {
			t.Fatalf("pill %s has no rect/button", sectionLabel(pill.id))
		}
	}
	pos := map[synthSectionID]int{}
	for i, id := range unifiedSynthSectionOrder {
		pos[id] = i
	}
	prev := -1
	for _, pill := range g.drum.instEditorPipePills {
		if pos[pill.id] < prev {
			t.Fatalf("strip out of pipeline order at %s", sectionLabel(pill.id))
		}
		prev = pos[pill.id]
	}
	_, off := g.drum.synp().activePipelineStages(roster.Violin)
	if len(off) == 0 {
		t.Fatal("expected off stages")
	}
	if add := g.drum.instEditorPipeAddBtn; add == nil || add.Rect().Empty() {
		t.Fatal("+ pill missing while stages are off")
	}
	// The strip never overlaps the cards.
	for _, c := range g.drum.instEditorMacroCards {
		if c.r.Overlaps(g.drum.instEditorPipeRect) {
			t.Fatalf("card %s %v overlaps strip %v", c.id, c.r, g.drum.instEditorPipeRect)
		}
	}
}

// A real tap on a stage pill (through Game.Update + the tree dispatcher)
// drills into that stage; the drilled view lists the stage's knobs and the
// strip is gone.
func TestPipelineStrip_TapDrillsIntoStage(t *testing.T) {
	g := newViolinMacroGame(t)
	layoutSynthFor(t, g, channel.Of(roster.Violin))
	g.Update()
	pill, ok := pipePillFor(g, synthSectionFilter)
	if !ok {
		t.Fatal("no FILTER pill")
	}
	clickAt(g, pill.rect.Min.X+2, pill.rect.Min.Y+2)
	tgt := g.drum.synthDrillTarget(roster.Violin)
	if tgt.kind != drillStage || tgt.stage != synthSectionFilter {
		t.Fatalf("drill target %+v, want FILTER stage", tgt)
	}
	layoutSynthFor(t, g, channel.Of(roster.Violin))
	if len(g.drum.instEditorPipePills) != 0 {
		t.Fatal("strip still laid out while drilled")
	}
	ess, _ := g.drum.drilledKnobIdxs(tgt)
	if len(ess) == 0 {
		t.Fatal("drilled FILTER stage has no essentials")
	}
	seen := false
	for _, k := range ess {
		if g.drum.instEditorBindings[k].def.Name == "filter_cutoff" && !g.drum.instEditorControls[k].Rect().Empty() {
			seen = true
		}
	}
	if !seen {
		t.Fatal("filter_cutoff knob not laid out in the drilled stage")
	}
	if b := g.drum.instEditorMacroBackBtn; b == nil || b.Rect().Empty() {
		t.Fatal("Back button missing in drilled view")
	}
	if g.drum.instEditorMacroPillRect.Empty() {
		t.Fatal("FILTER stage drill should show its enable pill")
	}
}

// Back returns to the root through the real button path.
func TestPipelineStrip_BackReturnsToRoot(t *testing.T) {
	g := newViolinMacroGame(t)
	layoutSynthFor(t, g, channel.Of(roster.Violin))
	g.drum.setSynthStageOpen(roster.Violin, synthSectionFilter)
	layoutSynthFor(t, g, channel.Of(roster.Violin))
	g.Update()
	b := g.drum.instEditorMacroBackBtn
	if b == nil {
		t.Fatal("no Back button")
	}
	clickAt(g, b.Rect().Min.X+2, b.Rect().Min.Y+2)
	if !g.drum.synthAtMacroRoot(roster.Violin) {
		t.Fatalf("still drilled: %+v", g.drum.synthDrillTarget(roster.Violin))
	}
}
