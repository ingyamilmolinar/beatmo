//go:build test

package ui

import (
	"image"
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

func TestSplitSynthRightPane3_SharesAndOrder(t *testing.T) {
	r := image.Rect(0, 0, 280, 400)
	full, up, focus := splitSynthRightPane3(r)
	if full.Empty() || up.Empty() || focus.Empty() {
		t.Fatalf("tall pane must host all three cards: %v %v %v", full, up, focus)
	}
	if !(full.Max.Y < up.Min.Y && up.Max.Y < focus.Min.Y) {
		t.Fatalf("cards must stack full/up/focus: %v %v %v", full, up, focus)
	}
	if !(focus.Dy() > full.Dy() && full.Dy() > up.Dy()) {
		t.Fatalf("share order must be focus > full > up: %d %d %d", full.Dy(), up.Dy(), focus.Dy())
	}
	if focus.Max.Y != r.Max.Y || full.Min.Y != r.Min.Y {
		t.Fatalf("cards must tile the pane")
	}
}

func TestSplitSynthRightPane3_UpCloseCollapsesFirst(t *testing.T) {
	minBand := Profile().DensityValues().SynthRightCardMinH
	r := image.Rect(0, 0, 280, 3*minBand-1)
	full, up, focus := splitSynthRightPane3(r)
	if !up.Empty() {
		t.Fatalf("short pane must collapse Up-close first, got %v", up)
	}
	if full.Empty() || focus.Empty() {
		t.Fatalf("two-card fallback must keep full-note + focus: %v %v", full, focus)
	}
}
