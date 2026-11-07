//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Centre and neighbour labels (through drawTextStyledHook) and at least one filled rect inside the lane.
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

	// So the renderer is not chrome-only.
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

// Offsets past the bounds draw no text.
func TestDrawBarrelLane_EmptyLabelDrawsNothing(t *testing.T) {
	g := newModularSynthTabGame(t)
	layoutSynthTab(t, g)
	dv := g.drum
	inst := dv.synthTabActiveInstrument()

	// An enum walks past its ends, where labelAt returns "".
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
	// Enough rows that some offsets fall outside the range.
	drawBarrelLane(dst, e.ValueLaneRect(), e.value(), synthValueBinding{dv: dv, idx: idx, instID: inst},
		barrelLaneStyle{RowH: 26, VisibleRows: 5, Accent: colConceptStroke, Emphasis: true})

	for _, s := range texts {
		if s == "" {
			t.Fatalf("empty label should never reach DrawTextStyled, got texts=%v", texts)
		}
	}
}
