//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestDrawSynthTab_RendersFocusGraphInLowerBand(t *testing.T) {
	g := newModularSynthTabGame(t)
	layoutSynthTab(t, g)
	dv := g.drum
	if dv.instEditorPreviewRect.Empty() {
		t.Skip("no desktop preview pane in this profile")
	}
	_, focus := splitSynthRightPane(dv.instEditorPreviewRect)
	if focus.Empty() {
		t.Fatalf("focus band empty for preview rect %v", dv.instEditorPreviewRect)
	}
	inst := dv.resolveSynthInstrument(dv.synthTabActiveInstrument())

	// The real content rect the EQ panel hands to DrawSynthTab.
	contentR := g.drum.eqPanelZone.ContentRect()

	// Count ink that lands inside the focus band when the whole tab draws.
	rects := collectFilledRects(t, func() {
		dv.drawSynthTab(ebiten.NewImage(g.winW, g.winH), contentR, inst)
	})
	ink := 0
	for _, r := range rects {
		if r.Rect.In(focus) {
			ink++
		}
	}
	if ink == 0 {
		t.Fatalf("no ink painted inside the focus band — focus graph not wired into drawSynthTab")
	}
}
