//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Fails when no such knob exists.
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

// Through the real layout, the knob fills its cell with no band beneath.
func TestSynthControlCellReservesNoVizBand(t *testing.T) {
	g := setupModularSynthGame(t)
	// A degenerate panel would shrink cells to nothing and make this vacuous.
	expandSynthPanelForTest(t, g)

	secID, _ := synthSectionContaining(t, g, "filter")
	g.drum.setSelectedSynthSection(roster.Modular, secID)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())

	dv2 := Profile().DensityValues()

	grid := g.drum.sectionGrid(secID)
	cell, vis := grid.CellRect(0)
	if !vis {
		t.Fatalf("first knob cell of section %v is not visible after layout", secID)
	}

	// Its rect spans dial and caption; below it, only the inter-row gap.
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
	knobR := g.drum.instEditorControls[knobIdx].Rect()
	if knobR.Empty() {
		t.Fatalf("first knob of section %v has empty rect", secID)
	}
	band := cell.Max.Y - knobR.Max.Y
	if band > SpaceXS {
		t.Fatalf("knob cell reserves %d px below knob (cell=%v knob=%v) — Task 8 removed the viz band; expected <= %d (gap only)",
			band, cell, knobR, SpaceXS)
	}
	// And the dial must not have been enlarged past its ideal diameter.
	dialDia := knobR.Dy() - dv2.SynthControlCaptionH
	if dialDia > dv2.SynthControlIdeal {
		t.Errorf("dial diameter %d exceeds SynthControlIdeal %d — cell growth wrongly enlarged the dial", dialDia, dv2.SynthControlIdeal)
	}
}

// No picture is drawn below a knob's caption.
func TestSynthControlDrawsNoConceptBand(t *testing.T) {
	g := setupModularSynthGame(t)
	expandSynthPanelForTest(t, g)

	secID, _ := synthSectionContaining(t, g, "filter")
	g.drum.setSelectedSynthSection(roster.Modular, secID)
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
	knobR := g.drum.instEditorControls[knobIdx].Rect()

	// No reserved viz band: the gap below the knob is at most the inter-row gap.
	band := cell.Max.Y - knobR.Max.Y
	if band > SpaceXS {
		t.Fatalf("knob cell reserves %d px below knob (cell=%v knob=%v) — Task 8 removed the concept band; expected <= %d (gap only)",
			band, cell, knobR, SpaceXS)
	}

	// And the detail pane still draws fine (no panic, produces chrome ink).
	scratch := ebiten.NewImage(640, 480)
	rects := collectFilledRects(t, func() {
		g.drum.drawSynthDetailPane(scratch, roster.Modular)
	})
	if len(rects) == 0 {
		t.Fatalf("synth detail pane drew nothing")
	}
}

// Empty for unmapped labels.
func TestSynthControlPurpose_Gloss(t *testing.T) {
	if got := synthControlPurpose(audio.ParamDef{Label: "Cutoff"}); got != "bright or dull" {
		t.Errorf("Cutoff gloss = %q, want %q", got, "bright or dull")
	}
	if got := synthControlPurpose(audio.ParamDef{Label: "Resonance"}); got != "ringing edge" {
		t.Errorf("Resonance gloss = %q, want %q", got, "ringing edge")
	}
	// Falls back to Name when Label has no mapping.
	if got := synthControlPurpose(audio.ParamDef{Label: "Zzz Unmapped", Name: "drive"}); got != "softness vs edge" {
		t.Errorf("Name fallback gloss = %q, want %q", got, "softness vs edge")
	}
	if got := synthControlPurpose(audio.ParamDef{Label: "Zzz", Name: "qqq"}); got != "" {
		t.Errorf("unmapped gloss = %q, want empty", got)
	}
}

// As many columns as the pane fits (up to synthMaxGridCols).
func TestSynthSectionGridFlowsColumnsDesktop(t *testing.T) {
	g := setupModularSynthGame(t)
	expandSynthPanelForTest(t, g)

	// Pick a section with more than two knobs so the wide pane can fit >2 cards.
	var secID synthSectionID = -1
	for _, s := range g.drum.SynthTabSections() {
		if len(s.knobIdxs) > 2 {
			secID = s.id
			break
		}
	}
	if secID < 0 {
		t.Skip("no modular section with >2 knobs to exercise the column flow")
	}
	g.drum.setSelectedSynthSection(roster.Modular, secID)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())

	if cols := g.drum.sectionGrid(secID).Cols(); cols <= 2 {
		t.Errorf("desktop synth section %v laid out %d columns, want > 2 (card cells flow, no cap)", secID, cols)
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
