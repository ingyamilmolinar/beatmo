//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// synth_armed_anim_test.go — Task 12 verification: the per-frame animation
// tick + render-cache bypass for the ARMED synth control were already wired
// in by Tasks 7 & 10 (synth_panel_zone.go: synthTabUpdate's e.Tick() call;
// drawSynthDetailPane's "continue" before p.sprites.card(...) for the armed
// idx). These two tests formalize that contract as regression guards so a
// future refactor can't silently reintroduce a frozen/cached armed card.

// TestArmedEditorTicksUntilSettled pins the tick contract: while the armed
// control's kinetic barrel wheel is still moving, synthTabUpdate() must
// report changed=true (driving a redraw every frame); once the wheel settles
// it must go back to false. Without the Tick() call inside synthTabUpdate,
// an impulse given via keyboard/wheel input would freeze mid-animation the
// next time Update() ran without a fresh input event.
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

// TestArmedCardSkipsPerCardSpriteCache is the regression guard for the
// render-cache bypass half of Task 12. drawSynthDetailPane's armed branch
// (synth_panel_zone.go, just above "Task 7: the armed control's grown card
// renders the two kinetic barrel lanes...") calls p.drawArmedControlCard
// directly and `continue`s BEFORE reaching p.sprites.card(b.def.Name) — the
// armed control's card sprite is therefore never created and never cached. A
// future refactor that routed the armed card back through the per-card
// TraceSprite cache would freeze the kinetic lanes mid-scroll (a cached
// sprite only repaints when its key changes; the barrel lanes need to repaint
// every animating frame even though the key components — name/value/gauge —
// can be momentarily identical across two sub-detent ticks).
//
// Asserts both:
//  1. structurally — the armed control's param name never appears as a key
//     in synthSpriteCache.cards after a Draw, proving the cache path was
//     skipped entirely (not merely invalidated every frame, which would also
//     leave a stale entry key present).
//  2. behaviorally — nudging the wheel past a full detent and redrawing
//     changes the barrel's rendered labels on the very next Draw call. A
//     cached sprite would reproduce byte-identical output until its key
//     (which the armed path never even sets) changed.
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

	// Structural: run the REAL dispatch path (drawSynthDetailPane, at the
	// fixture's natural 640×480 layout) and confirm the armed control's
	// param name never lands in the per-card sprite cache — proves the
	// "continue" branch (Task 7, synth_panel_zone.go) skips
	// p.sprites.card(b.def.Name) entirely for the armed idx rather than
	// merely invalidating a cache entry that still gets created.
	dst := ebiten.NewImage(640, 480)
	dv.drawSynthDetailPane(dst, inst)
	if sp, ok := dv.synp().sprites.cards[b.def.Name]; ok {
		t.Fatalf("armed control %q went through the per-card sprite cache (%v) — drawSynthDetailPane must draw the armed card directly to dst, bypassing p.sprites.card()", b.def.Name, sp)
	}

	// Behavioral: call drawArmedControlCard directly (the function
	// drawSynthDetailPane's armed branch dispatches straight to) with a
	// generous manually-set rect — mirrors synth_barrel_render_test.go,
	// since the fixture's real layout squeezes knob rects far too tight to
	// exercise the barrel lanes meaningfully. Because this path paints
	// straight to dst with no TraceSprite key-comparison in front of it, two
	// back-to-back calls after the wheel has moved a full detent must
	// produce different labels — a cached sprite would instead reproduce
	// byte-identical output for an unchanged key.
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

	// Nudge the wheel over a full detent (pick whichever direction the
	// current value can still move, so this isn't flaky against a control
	// parked at its min/max) and let it animate to the new detent.
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
