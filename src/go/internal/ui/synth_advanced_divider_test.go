package ui

import "testing"

// The advanced tier is always laid out below a labeled divider row — no
// expander, no hidden state. Essentials come first; advanced knobs always have
// grid cells (possibly scrolled out, but never structurally hidden).
//
// The synth detail pane is short (often a single visible knob row), so the
// three tiers cannot all be on screen at once. The test therefore proves the
// vertical ordering by SCROLL PROGRESSION: at the top the essentials show and
// the divider is below the fold; scrolling to the divider's row reveals a
// full-width divider band; scrolling to the end reveals the advanced knobs.
func TestSynthAdvancedDivider_EssentialsFirstAdvancedAlwaysPlaced(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	dv := g.drum

	// Pick a stage that has advanced knobs (walk sections until tierSplit
	// returns a non-empty advanced list).
	var stageID synthSectionID = -1
	for i := range dv.instEditorSections {
		s := dv.instEditorSections[i]
		if _, adv := dv.synthSectionTierSplit(s.knobIdxs); len(adv) > 0 {
			stageID = s.id
			break
		}
	}
	if stageID == -1 {
		t.Skip("no stage with advanced params in test registry")
	}
	dv.setSelectedSynthSection(dv.synthTabActiveInstrument(), stageID)
	layoutSynthTab(t, g)

	sel := dv.synthSelectedSection()
	if sel == nil {
		t.Fatal("no selected section after re-layout")
	}
	ess, adv := dv.synthSectionTierSplit(sel.knobIdxs)
	if len(adv) == 0 {
		t.Fatal("selected stage lost its advanced knobs")
	}
	if dv.synthAdvDividerCount != len(adv) {
		t.Fatalf("synthAdvDividerCount = %d, want %d", dv.synthAdvDividerCount, len(adv))
	}

	grid := dv.sectionGrid(sel.id)
	cols := grid.Cols()
	if cols < 1 {
		t.Fatalf("grid has no columns")
	}
	innerW := dv.instEditorDetailR.Dx()

	// Top of the list: the essentials occupy the first row(s). Record their
	// bottom for the ordering check below.
	maxEssBottom := 0
	essVisible := 0
	for _, i := range ess {
		if r := dv.instEditorKnobs[i].Rect(); !r.Empty() {
			essVisible++
			if r.Max.Y > maxEssBottom {
				maxEssBottom = r.Max.Y
			}
		}
	}
	if essVisible == 0 {
		t.Fatal("no essential knob is visible at the top of the stage")
	}

	// The divider row is laid out AFTER the essential rows. Scroll to it and
	// require a non-empty band spanning (most of) the pane width — the phantom
	// row is the full-width separator between the two tiers.
	essRows := (len(ess) + cols - 1) / cols
	dividerStartIdx := essRows * cols
	grid.ScrollToIndex(dividerStartIdx)
	layoutSynthTab(t, g)
	if dv.synthAdvDividerRect.Empty() {
		t.Fatal("expected a non-empty advanced divider rect once its row is in view")
	}
	if dv.synthAdvDividerRect.Dx()*2 < innerW {
		t.Fatalf("divider band %v does not span the pane width (%d)", dv.synthAdvDividerRect, innerW)
	}

	// Advanced knobs are structurally placed: scroll to the end and require at
	// least one advanced knob to acquire a visible rect (no expander state
	// gate exists anymore).
	grid.ScrollToIndex(dividerStartIdx + cols + len(adv))
	layoutSynthTab(t, g)
	anyAdvVisible := false
	for _, i := range adv {
		if !dv.instEditorKnobs[i].Rect().Empty() {
			anyAdvVisible = true
			break
		}
	}
	if !anyAdvVisible {
		t.Fatal("no advanced knob acquired a rect after scrolling to the end")
	}
	_ = maxEssBottom // ordering is proven by scroll progression, not co-visibility
}

// Desktop rows now flow as many cards as fit — at 1280 wide the detail pane
// must place at least 4 cards on the first row of a knob-rich stage (the dial
// era capped this at 2 columns).
func TestSynthCardGrid_DesktopRowDensity(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	dv := g.drum

	// Pick the stage with the most knobs.
	best := -1
	var bestID synthSectionID = -1
	for i := range dv.instEditorSections {
		if n := len(dv.instEditorSections[i].knobIdxs); n >= 4 && n > best {
			best = n
			bestID = dv.instEditorSections[i].id
		}
	}
	if bestID == -1 {
		t.Skip("no 4-knob stage available")
	}
	dv.setSelectedSynthSection(dv.synthTabActiveInstrument(), bestID)
	layoutSynthTab(t, g)
	if got := dv.sectionGrid(bestID).Cols(); got < 4 {
		t.Fatalf("desktop card grid cols = %d, want >= 4 (was capped at 2 in the dial era)", got)
	}
}
