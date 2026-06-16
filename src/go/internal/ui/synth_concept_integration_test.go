//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// synthSectionContaining returns the section id that owns a knob whose param
// def is in the given group, plus the binding index of that knob. Fails when no
// such knob exists in the laid-out section model.
func synthSectionContaining(t *testing.T, g *Game, group string) (synthSectionID, int) {
	t.Helper()
	bindings := g.drum.SynthTabBindings()
	for _, s := range g.drum.SynthTabSections() {
		for _, kIdx := range s.knobIdxs {
			if kIdx >= 0 && kIdx < len(bindings) && bindings[kIdx].def.Group == group {
				return s.id, kIdx
			}
		}
	}
	t.Fatalf("no knob in group %q across modular synth sections", group)
	return -1, -1
}

// TestSynthKnobCellReservesNoVizBand drives the real Synth-tab layout for the
// modular instrument and asserts each laid-out knob CELL no longer reserves a
// concept-viz band below the knob+caption. Post-Task-8 the per-knob mini-picture
// is gone (the right-pane focus graph explains the selected knob), so the cellH
// formula drops SynthConceptVizH and the dial+caption fill the cell.
func TestSynthKnobCellReservesNoVizBand(t *testing.T) {
	g := setupModularSynthGame(t)
	// Give the detail pane real vertical room so cells lay out (a degenerate
	// panel would shrink cellH to nothing and the assertion would be vacuous).
	expandSynthPanelForTest(t, g)

	secID, _ := synthSectionContaining(t, g, "filter")
	g.drum.setSelectedSynthSection("modular", secID)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())

	dv2 := Profile().DensityValues()

	grid := g.drum.sectionGrid(secID)
	cell, vis := grid.CellRect(0)
	if !vis {
		t.Fatalf("first knob cell of section %v is not visible after layout", secID)
	}

	// The first knob of the section: its laid-out Rect() includes the dial +
	// caption row. The space between its bottom and the cell bottom must now be
	// no more than the inter-row gap — there is NO reserved viz band any more.
	var knobIdx int = -1
	for _, s := range g.drum.SynthTabSections() {
		if s.id == secID && len(s.knobIdxs) > 0 {
			knobIdx = s.knobIdxs[0]
			break
		}
	}
	if knobIdx < 0 {
		t.Fatalf("section %v has no knobs", secID)
	}
	knobR := g.drum.instEditorKnobs[knobIdx].Rect()
	if knobR.Empty() {
		t.Fatalf("first knob of section %v has empty rect", secID)
	}
	band := cell.Max.Y - knobR.Max.Y
	if band > SpaceXS {
		t.Fatalf("knob cell reserves %d px below knob (cell=%v knob=%v) — Task 8 removed the viz band; expected <= %d (gap only)",
			band, cell, knobR, SpaceXS)
	}
	// And the dial must not have been enlarged past its ideal diameter.
	dialDia := knobR.Dy() - dv2.SynthKnobCaptionH
	if dialDia > dv2.SynthKnobIdeal {
		t.Errorf("dial diameter %d exceeds SynthKnobIdeal %d — cell growth wrongly enlarged the dial", dialDia, dv2.SynthKnobIdeal)
	}
}

// TestSynthKnobDrawsNoConceptBand renders the synth detail pane and asserts
// the per-knob concept picture is no longer drawn below the knob's caption.
// Post-Task-8 the right-pane focus graph explains the selected knob, so the
// in-cell mini-picture (and its reserved band) are gone. The concept RENDERERS
// themselves still work (see TestEveryKnobHasConceptInk /
// TestConceptKnobWave_RendersAndMorphs); they're just no longer wired into the
// per-knob cell. Here we assert the cell reserves no band below the knob.
func TestSynthKnobDrawsNoConceptBand(t *testing.T) {
	g := setupModularSynthGame(t)
	expandSynthPanelForTest(t, g)

	secID, _ := synthSectionContaining(t, g, "filter")
	g.drum.setSelectedSynthSection("modular", secID)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())

	grid := g.drum.sectionGrid(secID)
	cell, vis := grid.CellRect(0)
	if !vis {
		t.Fatalf("first knob cell of section %v is not visible after layout", secID)
	}
	var knobIdx int = -1
	for _, s := range g.drum.SynthTabSections() {
		if s.id == secID && len(s.knobIdxs) > 0 {
			knobIdx = s.knobIdxs[0]
			break
		}
	}
	if knobIdx < 0 {
		t.Fatalf("section %v has no knobs", secID)
	}
	knobR := g.drum.instEditorKnobs[knobIdx].Rect()

	// No reserved viz band: the gap below the knob is at most the inter-row gap.
	band := cell.Max.Y - knobR.Max.Y
	if band > SpaceXS {
		t.Fatalf("knob cell reserves %d px below knob (cell=%v knob=%v) — Task 8 removed the concept band; expected <= %d (gap only)",
			band, cell, knobR, SpaceXS)
	}

	// And the detail pane still draws fine (no panic, produces chrome ink).
	scratch := ebiten.NewImage(640, 480)
	rects := collectFilledRects(t, func() {
		g.drum.drawSynthDetailPane(scratch, "modular")
	})
	if len(rects) == 0 {
		t.Fatalf("synth detail pane drew nothing")
	}
}

// TestSynthKnobPurpose_Gloss — the per-knob purpose line resolves a kid-readable
// gloss from the param Label (or Name), and is empty for unmapped labels.
func TestSynthKnobPurpose_Gloss(t *testing.T) {
	if got := synthKnobPurpose(audio.ParamDef{Label: "Cutoff"}); got != "bright or dull" {
		t.Errorf("Cutoff gloss = %q, want %q", got, "bright or dull")
	}
	if got := synthKnobPurpose(audio.ParamDef{Label: "Resonance"}); got != "ringing edge" {
		t.Errorf("Resonance gloss = %q, want %q", got, "ringing edge")
	}
	// Falls back to Name when Label has no mapping.
	if got := synthKnobPurpose(audio.ParamDef{Label: "Zzz Unmapped", Name: "drive"}); got != "softness vs edge" {
		t.Errorf("Name fallback gloss = %q, want %q", got, "softness vs edge")
	}
	if got := synthKnobPurpose(audio.ParamDef{Label: "Zzz", Name: "qqq"}); got != "" {
		t.Errorf("unmapped gloss = %q, want empty", got)
	}
}

// TestSynthSectionGridCapsColumnsDesktop — the synth section grid is capped to
// 2 columns on desktop so each knob cell is wide enough for its purpose line +
// concept picture, even when the pane is wide enough to fit more.
func TestSynthSectionGridCapsColumnsDesktop(t *testing.T) {
	g := setupModularSynthGame(t)
	expandSynthPanelForTest(t, g)

	// Pick a section with more than two knobs so an uncapped grid WOULD use >2
	// columns on a wide pane — proving the cap is what holds it to 2.
	var secID synthSectionID = -1
	for _, s := range g.drum.SynthTabSections() {
		if len(s.knobIdxs) > 2 {
			secID = s.id
			break
		}
	}
	if secID < 0 {
		t.Skip("no modular section with >2 knobs to exercise the column cap")
	}
	g.drum.setSelectedSynthSection("modular", secID)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())

	if cols := g.drum.sectionGrid(secID).Cols(); cols > 2 {
		t.Errorf("desktop synth section %v laid out %d columns, want <= 2 (fewer-per-row cap)", secID, cols)
	}
}

// mapsEqualRectCount reports whether two rect→count multisets are identical.
func mapsEqualRectCount(a, b map[image.Rectangle]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
