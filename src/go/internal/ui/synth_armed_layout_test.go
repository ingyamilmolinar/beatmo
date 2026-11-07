package ui

import "testing"

// The 640x480 fixture's detail pane is ~10-60 px tall, leaving a grown card no room to grow into; a resize forces
// a genuine relayout at a realistic height.
func growSynthPanelForTest(t *testing.T, g *Game) {
	t.Helper()
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.Layout(1280, 2000)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	layoutSynthTab(t, g)
}

// The armed card grows while unarmed cells keep the uniform slot height.
func TestArmedCardGrowsInPlace(t *testing.T) {
	g := newModularSynthTabGame(t)
	dv := g.drum
	inst := dv.synthTabActiveInstrument()

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
	// About two cell rows tall, with slack for the gap.
	if armedRect.Dy() < 2*restH-8 {
		t.Fatalf("armed height should be ~2× a resting cell (%d), got %d", 2*restH, armedRect.Dy())
	}
}

// The Advanced divider shifts with the cells below the armed row, or its label overlaps the rows it separates;
// OSC carries an Advanced param, so its band exists here.
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

	// Full width, never riding up into the armed block; it moves down only when the block overflows the last
	// essential row.
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
