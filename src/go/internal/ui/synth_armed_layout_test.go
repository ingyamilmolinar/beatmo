package ui

import "testing"

// growSynthPanelForTest gives the Synth tab's detail pane genuine vertical
// headroom (well beyond SynthArmedCardH), unlike the bare 640x480 fixture
// (newSynthTabGame's default) where the detail pane's padded content rect is
// only ~10-60px tall — every knob cell already fills 100% of the available
// room there, so a grown armed card has no headroom to grow INTO regardless
// of the growth logic, and the assertion would be physically unsatisfiable.
// A real desktop audio panel always has far more vertical room than
// SynthArmedCardH; this mirrors expandSynthPanelForTest's "resize forces a
// genuine relayout" trick (synth_control_adapter_index_safe_test.go) at a
// taller target so the detail pane clears SynthArmedCardH + header + padding.
func growSynthPanelForTest(t *testing.T, g *Game) {
	t.Helper()
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.Layout(1280, 2000)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	layoutSynthTab(t, g)
}

// TestArmedCardGrowsInPlace proves Task 7's post-slot height extension: the
// armed control's card grows to SynthArmedCardH while the un-armed default
// layout keeps the uniform ControlGrid slot height.
func TestArmedCardGrowsInPlace(t *testing.T) {
	g := newModularSynthTabGame(t)
	dv := g.drum
	inst := dv.synthTabActiveInstrument()

	// Lay out once un-armed (at a realistic panel size); capture control 0's
	// height.
	growSynthPanelForTest(t, g)
	restRect := dv.instEditorControls[0].Rect()
	restH := restRect.Dy()
	if restH <= 0 {
		t.Fatalf("control 0 rect should be non-empty before arming, got %v", restRect)
	}

	dv.armSynthControl(inst, 0)
	for i := 0; i < 50 && dv.advanceArmGrow(); i++ {
		layoutSynthTab(t, g)
	}
	layoutSynthTab(t, g)
	armedRect := dv.instEditorControls[0].Rect()
	if armedRect.Dy() <= restH {
		t.Fatalf("armed card should be taller: rest=%d armed=%d", restH, armedRect.Dy())
	}
	// New 2×2 contract: the armed card grows to ~2 cell-rows tall (2× height), not
	// the legacy SynthArmedCardH token. Allow slack for the inter-row gap.
	if armedRect.Dy() < 2*restH-8 {
		t.Fatalf("armed height should be ~2× a resting cell (%d), got %d", 2*restH, armedRect.Dy())
	}
}

// TestArmedCardShiftsAdvancedDivider proves the Advanced-tier divider band
// (dv.synthAdvDividerRect) is re-shifted by the same delta growArmedCard
// applies to cells below the armed row — otherwise the "— ADVANCED · N —"
// label stays frozen at its pre-grow Y while the rows it separates move
// down, producing an overlap/gap. Control 0 on the modular test instrument
// is osc_type (OSC section, Essential tier); OSC also carries an Advanced
// param (osc_detune), so the section's divider band is non-empty here.
func TestArmedCardShiftsAdvancedDivider(t *testing.T) {
	g := newModularSynthTabGame(t)
	dv := g.drum
	inst := dv.synthTabActiveInstrument()

	growSynthPanelForTest(t, g)
	if dv.instEditorBindings[0].def.Group != "osc" {
		t.Fatalf("expected control 0 in group osc, got %q", dv.instEditorBindings[0].def.Group)
	}
	before := dv.synthAdvDividerRect
	if before.Empty() {
		t.Fatalf("expected a non-empty advanced-tier divider before arming")
	}

	dv.armSynthControl(inst, 0)
	for i := 0; i < 50 && dv.advanceArmGrow(); i++ {
		layoutSynthTab(t, g)
	}
	layoutSynthTab(t, g)
	after := dv.synthAdvDividerRect
	if after.Empty() {
		t.Fatalf("expected a non-empty advanced-tier divider after arming")
	}

	// New reflow contract: the divider stays a full-width band (its X span is
	// preserved) and never rides UP into the armed block. With the masonry it only
	// moves down when the block's extra cells overflow the last essential row —
	// otherwise it holds. The invariant that matters is no-overlap (below), plus
	// the band staying full width.
	if after.Min.X != before.Min.X || after.Max.X != before.Max.X {
		t.Fatalf("divider should stay full-width: before X [%d,%d] after X [%d,%d]",
			before.Min.X, before.Max.X, after.Min.X, after.Max.X)
	}
	if after.Min.Y < before.Min.Y {
		t.Fatalf("divider must not ride UP past its resting row: before=%v after=%v", before, after)
	}
	armedRect := dv.instEditorControls[0].Rect()
	if after.Min.Y < armedRect.Max.Y {
		t.Fatalf("divider overlaps the grown armed card: divider=%v armedRect=%v", after, armedRect)
	}
}
