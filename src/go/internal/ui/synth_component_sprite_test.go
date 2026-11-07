//go:build test

package ui

import (
	"image"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// The graph sprite's eviction (tab hide, locale) and the per-param cards' pixel identity and render count.

// assertImagesEqual compares two equally-sized images pixel-for-pixel.
func assertImagesEqual(t *testing.T, a, b *ebiten.Image, w, h int) {
	t.Helper()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			pa, pb := a.At(x, y), b.At(x, y)
			if pa != pb {
				t.Fatalf("pixel mismatch at (%d,%d): cached=%v direct=%v", x, y, pa, pb)
			}
		}
	}
}

// ── eviction ─────────────────────────────────────────────────────────────

// Leaving the tab releases the graph texture at once; returning re-allocates on the next Draw.
func TestSynthSpriteEvict_ReleasedOnTabHide(t *testing.T) {
	g := driveScene(t, "crop_synth_tab_modular")
	dv := g.drum
	if dv.instEditorPreviewRect.Empty() {
		t.Skip("no desktop preview pane in this profile")
	}
	screen := ebiten.NewImage(1280, 720)
	g.Draw(screen)

	// Re-read each call: the struct holds TraceSprite by value, so a copy taken before Release goes stale.
	resident := func() bool { return dv.synp().sprites.graph.resident() }
	inst := dv.resolveSynthInstrument(dv.synthTabActiveInstrument())
	dv.setSynthStageOpen(inst, dv.instEditorSections[0].id)
	g.Update()
	g.Draw(screen)
	if !resident() {
		t.Fatal("synth tab draw did not allocate its graph sprite")
	}

	g.drum.eqPanelZone.SetActiveTab(TabWave)
	if resident() {
		t.Fatal("leaving TabSynth must release its graph sprite")
	}

	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.Draw(screen)
	if !resident() {
		t.Fatal("returning to TabSynth must re-allocate the graph sprite on draw")
	}
}

// Locale-blind keys, so a language switch must release them or they show the old language.
func TestSynthSprites_ReleasedOnLocaleChange(t *testing.T) {
	g := driveScene(t, "crop_synth_tab_modular")
	dv := g.drum
	screen := ebiten.NewImage(1280, 720)
	g.Draw(screen) // warm sprites on the active Synth tab

	inst := dv.resolveSynthInstrument(dv.synthTabActiveInstrument())
	dv.setSynthStageOpen(inst, dv.instEditorSections[0].id)
	g.Update()
	g.Draw(screen)
	sp := dv.synp()
	if !sp.sprites.graph.resident() {
		t.Fatal("synth tab draw did not allocate the graph sprite — cannot test locale eviction")
	}

	dv.OnLocaleChanged()

	if sp.sprites.graph.resident() {
		t.Fatal("OnLocaleChanged must release synth sprites so localized text re-renders in the new locale")
	}
}

// Each card has its own sprite keyed by paramID: a steady frame blits N textures, an edit re-renders the one card
// whose key changed.

// Bound to the modular voice, with the selected section's cards laid out.
func newSynthTabForCardSprites(t *testing.T) (dv *DrumView, instID string) {
	t.Helper()
	g := newModularSynthTabGame(t)
	expandEQPanelForTest(t, g)
	layoutSynthTab(t, g)
	dv = g.drum
	instID = string(dv.resolveSynthInstrument(dv.synthTabActiveInstrument()))
	if instID == "" {
		t.Fatal("no active synth instrument")
	}
	return dv, instID
}

// The exact key the production loop builds, not a stand-in.
func cardKeyFor(dv *DrumView, instID string, sel *synthSection, kIdx int) (paramID, name, value string, gauge paramCardGauge, st paramCardState, key any) {
	k := dv.instEditorControls[kIdx]
	b := dv.instEditorBindings[kIdx]
	st = paramCardState{
		Selected: dv.synthSelectedControlIdx(instrument.ID(instID), sel) == kIdx,
		Dim:      false,
	}
	name = synthParamDisplayName(b.def.Name)
	value = knobPillLabel(k, b.def)
	gauge = paramCardGaugeFor(k, b.def)
	key = struct {
		name, value string
		gauge       paramCardGauge
		st          paramCardState
	}{name, value, gauge, st}
	return b.def.Name, name, value, gauge, st, key
}

// The cached and RENDER_SAFE paths give identical pixels.
func TestSynthParamCardSprite_MatchesDirect(t *testing.T) {
	dv, instID := newSynthTabForCardSprites(t)
	p := dv.synp()
	sel := dv.synthSelectedSection()
	if sel == nil || len(sel.knobIdxs) == 0 {
		t.Skip("selected section has no knobs")
	}
	kIdx := sel.knobIdxs[0]
	if kIdx < 0 || kIdx >= len(dv.instEditorControls) || dv.instEditorControls[kIdx].Rect().Empty() {
		t.Skip("first card not visible in this profile")
	}
	rOrig := dv.instEditorControls[kIdx].Rect()
	paramID, name, value, gauge, st, key := cardKeyFor(dv, instID, sel, kIdx)

	r := image.Rect(0, 0, rOrig.Dx(), rOrig.Dy())
	cached := ebiten.NewImage(r.Dx(), r.Dy())
	p.sprites.card(paramID).Draw(cached, r, key, func(img *ebiten.Image, rr image.Rectangle) {
		drawParamCard(img, rr, name, value, gauge, st)
	})

	old := envRenderSafe
	envRenderSafe = true
	direct := ebiten.NewImage(r.Dx(), r.Dy())
	p.sprites.card(paramID).Draw(direct, r, key, func(img *ebiten.Image, rr image.Rectangle) {
		drawParamCard(img, rr, name, value, gauge, st)
	})
	envRenderSafe = old

	assertImagesEqual(t, cached, direct, r.Dx(), r.Dy())
}

// Identical frames re-render nothing; editing one card re-renders that card alone, its siblings stay hits.
func TestSynthParamCards_OnlyEditedCardReRenders(t *testing.T) {
	dv, instID := newSynthTabForCardSprites(t)
	p := dv.synp()
	sel := dv.synthSelectedSection()
	if sel == nil {
		t.Skip("no selected section")
	}
	var visible []int
	var paramIDs []string
	for _, kIdx := range sel.knobIdxs {
		if kIdx < 0 || kIdx >= len(dv.instEditorControls) {
			continue
		}
		if !dv.instEditorControls[kIdx].Rect().Empty() {
			visible = append(visible, kIdx)
			paramIDs = append(paramIDs, dv.instEditorBindings[kIdx].def.Name)
		}
	}
	if len(visible) < 2 {
		t.Skip("need >=2 visible cards to exercise per-card cache isolation")
	}

	dst := ebiten.NewImage(1280, 720)

	// The real loop: if cards do not go through p.sprites.card, no entry exists and the test fails.
	p.drawSynthDetailPane(dst, instrument.ID(instID))

	sprites := make([]*TraceSprite, len(visible))
	for i, id := range paramIDs {
		s := p.sprites.cards[id]
		if s == nil || !s.resident() {
			t.Fatalf("drawSynthDetailPane did not sprite-wrap card %q (p.sprites.cards has no resident entry) — card loop not routed through p.sprites.card", id)
		}
		sprites[i] = s
	}

	// A fixed clock, so the 33 ms throttle never masks a re-render. The warm frame stamped real time, far ahead of it,
	// so Invalidate and one more render re-stamp lastRender under the injected clock.
	now := time.Unix(50_000, 0)
	clock := func() time.Time { return now }
	for _, s := range sprites {
		s.now = clock
		s.Invalidate()
	}

	snapshot := func() []time.Time {
		out := make([]time.Time, len(sprites))
		for i, s := range sprites {
			out[i] = s.lastRender
		}
		return out
	}

	now = now.Add(time.Second)
	p.drawSynthDetailPane(dst, instrument.ID(instID)) // re-primes lastRender under the injected clock
	before := snapshot()

	now = now.Add(time.Second)
	p.drawSynthDetailPane(dst, instrument.ID(instID))
	steady := snapshot()
	for i := range sprites {
		if steady[i] != before[i] {
			t.Fatalf("steady-state frame re-rendered card %q (want cached blit only)", paramIDs[i])
		}
	}

	// Until the derived key really changes (an enum dot may not move for every delta); no draw, so the render trail
	// stays clean.
	editIdx := -1
	for i, kIdx := range visible {
		k := dv.instEditorControls[kIdx]
		b := dv.instEditorBindings[kIdx]
		gaugeBefore := paramCardGaugeFor(k, b.def)
		valueBefore := knobPillLabel(k, b.def)
		orig := k.Value
		k.Value = 1 - orig
		if paramCardGaugeFor(k, b.def) != gaugeBefore || knobPillLabel(k, b.def) != valueBefore {
			editIdx = i
			break
		}
		k.Value = orig
	}
	if editIdx < 0 {
		t.Skip("no visible card's key changes when its value flips — cannot exercise edit isolation")
	}

	now = now.Add(time.Second)
	p.drawSynthDetailPane(dst, instrument.ID(instID))
	edited := snapshot()
	for i := range sprites {
		if i == editIdx {
			if edited[i] == steady[i] {
				t.Fatalf("edited card %q did not re-render after its value changed (stale cache)", paramIDs[i])
			}
		} else if edited[i] != steady[i] {
			t.Fatalf("un-edited card %q re-rendered after sibling card %q was edited — cache is not isolated per paramID", paramIDs[i], paramIDs[editIdx])
		}
	}

	now = now.Add(time.Second)
	p.drawSynthDetailPane(dst, instrument.ID(instID))
	next := snapshot()
	for i := range sprites {
		if next[i] != edited[i] {
			t.Fatalf("post-edit steady frame re-rendered card %q (want cached blit only)", paramIDs[i])
		}
	}
}
