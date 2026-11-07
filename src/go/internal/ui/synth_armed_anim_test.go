//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// The armed control ticks every frame and bypasses the render cache, so it never freezes mid-animation.

// changed=true while the barrel moves, false once settled; without the Tick in synthTabUpdate an impulse freezes on
// the next input-free Update.
func TestArmedEditorTicksUntilSettled(t *testing.T) {
	g := newModularSynthTabGame(t)
	layoutSynthTab(t, g)
	dv := g.drum

	inst := dv.synthTabActiveInstrument()
	sel := dv.synthSelectedSection()
	if sel == nil || len(sel.knobIdxs) == 0 {
		t.Fatal("selected section has no knobs to arm")
	}
	idx := sel.knobIdxs[0]
	dv.armSynthControl(inst, idx)
	if dv.instEditorArmedEditor == nil {
		t.Fatal("armSynthControl did not create an ArmedControlEditor")
	}
	dv.instEditorArmedEditor.value().ImpulseNotch(+1)

	sawChange := false
	for i := 0; i < 300; i++ {
		if dv.synthTabUpdate() {
			sawChange = true
		} else if sawChange {
			return // settled after animating — good
		}
	}
	t.Fatal("armed editor animation should drive synthTabUpdate and then settle")
}

// The armed card is drawn directly, never through the per-card sprite cache, whose key can stay identical across
// two sub-detent ticks while the lanes must repaint. Checked structurally (no cache key) and behaviourally
// (labels change on the next Draw after a full detent).
func TestArmedCardSkipsPerCardSpriteCache(t *testing.T) {
	g := newModularSynthTabGame(t)
	layoutSynthTab(t, g)
	dv := g.drum

	inst := dv.synthTabActiveInstrument()
	sel := dv.synthSelectedSection()
	if sel == nil || len(sel.knobIdxs) == 0 {
		t.Fatal("selected section has no knobs to arm")
	}
	idx := sel.knobIdxs[0]
	dv.armSynthControl(inst, idx)
	e := dv.instEditorArmedEditor
	if e == nil {
		t.Fatal("armSynthControl did not create an ArmedControlEditor")
	}
	b := dv.instEditorBindings[idx]

	// The real dispatch at 640×480: the armed name never lands in the card cache, rather than being invalidated after
	// creation.
	dst := ebiten.NewImage(640, 480)
	dv.drawSynthDetailPane(dst, inst)
	if sp, ok := dv.synp().sprites.cards[b.def.Name]; ok {
		t.Fatalf("armed control %q went through the per-card sprite cache (%v) — drawSynthDetailPane must draw the armed card directly to dst, bypassing p.sprites.card()", b.def.Name, sp)
	}

	// drawArmedControlCard directly with a generous rect, since the fixture squeezes knobs too tight for the lanes;
	// with no sprite in front, two calls a detent apart must differ.
	origHook := drawTextStyledHook
	var texts []string
	drawTextStyledHook = func(s string) { texts = append(texts, s) }
	t.Cleanup(func() { drawTextStyledHook = origHook })

	cardR := image.Rect(0, 0, 120, 148)
	dv.synp().drawSynthArmedControlCard(dst, cardR, b, inst, idx)
	firstTexts := append([]string(nil), texts...)
	if len(firstTexts) < 2 {
		t.Fatalf("expected the armed card's header name plus at least one barrel label, got %v", firstTexts)
	}

	// Whichever direction the value can still move, so a control at its min/max is not flaky.
	dir := 1
	if !(synthValueBinding{dv: dv, idx: idx, instID: inst}).canStep(1) {
		dir = -1
	}
	e.value().ImpulseNotch(dir)
	for i := 0; i < 300 && e.Tick(); i++ {
	}

	texts = nil
	dv.synp().drawSynthArmedControlCard(dst, cardR, b, inst, idx)
	secondTexts := append([]string(nil), texts...)

	same := len(firstTexts) == len(secondTexts)
	if same {
		for i := range firstTexts {
			if firstTexts[i] != secondTexts[i] {
				same = false
				break
			}
		}
	}
	if same {
		t.Fatalf("armed card rendered identical labels after the wheel moved a full detent — looks frozen/cached: %v", firstTexts)
	}
}
