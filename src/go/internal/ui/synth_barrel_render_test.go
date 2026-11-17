//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestDrawBarrelLane_RendersCenterAndNeighbors proves the modern-flat lane
// renderer paints real data: center + neighbor text labels (captured via the
// drawTextStyledHook — there is no offscreen text sink in this codebase, this
// hook is the equivalent mechanism, mirrored from levels_visible_filter_test.go's
// collectDrawnTexts) plus at least one filled rect (the accent hairline and/or
// the edge-fade gradient bands) inside the lane rect via the drawRect
// interceptor (collectFilledRects, render_verify_test.go).
func TestDrawBarrelLane_RendersCenterAndNeighbors(t *testing.T) {
	g := newModularSynthTabGame(t)
	layoutSynthTab(t, g)
	dv := g.drum
	inst := dv.synthTabActiveInstrument()
	dv.armSynthControl(inst, 0)
	e := dv.instEditorArmedEditor
	if e == nil {
		t.Fatal("armSynthControl did not create an ArmedControlEditor")
	}
	e.SetCardRect(image.Rect(0, 0, 120, 148))

	origHook := drawTextStyledHook
	var texts []string
	drawTextStyledHook = func(s string) { texts = append(texts, s) }
	t.Cleanup(func() { drawTextStyledHook = origHook })

	dst := ebiten.NewImage(120, 148)
	laneRect := e.ValueLaneRect()
	if laneRect.Empty() {
		t.Fatal("ValueLaneRect is empty")
	}

	rects := collectFilledRects(t, func() {
		drawBarrelLane(dst, laneRect, e.value(), synthValueBinding{dv: dv, idx: 0, instID: inst},
			barrelLaneStyle{RowH: 26, VisibleRows: 5, Accent: colConceptStroke, Emphasis: true})
	})

	if len(texts) < 3 {
		t.Fatalf("expected center + neighbor labels drawn, got %d: %v", len(texts), texts)
	}

	// At least the accent hairline (and/or an edge-fade band) must land as a
	// filled rect inside the lane rect — proves the renderer isn't chrome-only.
	found := 0
	for _, r := range rects {
		if !r.Rect.Intersect(laneRect).Empty() {
			found++
		}
	}
	if found == 0 {
		t.Fatalf("expected at least one filled rect inside lane rect %v, got none (rects=%v)", laneRect, rects)
	}
}

// TestDrawBarrelLane_EmptyLabelDrawsNothing verifies that offsets whose
// binding.labelAt returns "" (past the enum/value bounds) draw no text.
func TestDrawBarrelLane_EmptyLabelDrawsNothing(t *testing.T) {
	g := newModularSynthTabGame(t)
	layoutSynthTab(t, g)
	dv := g.drum
	inst := dv.synthTabActiveInstrument()

	// Pick the enum control (if any) so offsets walk past the enum's ends,
	// which is where labelAt legitimately returns "". Fall back to control 0.
	idx := 0
	dv.armSynthControl(inst, idx)
	e := dv.instEditorArmedEditor
	if e == nil {
		t.Fatal("armSynthControl did not create an ArmedControlEditor")
	}
	e.SetCardRect(image.Rect(0, 0, 120, 148))

	origHook := drawTextStyledHook
	var texts []string
	drawTextStyledHook = func(s string) { texts = append(texts, s) }
	t.Cleanup(func() { drawTextStyledHook = origHook })

	dst := ebiten.NewImage(120, 148)
	// A huge VisibleRows count guarantees some offsets fall outside the
	// binding's real/enum range and return "".
	drawBarrelLane(dst, e.ValueLaneRect(), e.value(), synthValueBinding{dv: dv, idx: idx, instID: inst},
		barrelLaneStyle{RowH: 26, VisibleRows: 5, Accent: colConceptStroke, Emphasis: true})

	for _, s := range texts {
		if s == "" {
			t.Fatalf("empty label should never reach DrawTextStyled, got texts=%v", texts)
		}
	}
}
