//go:build test

package ui

import (
	"image"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// synth_component_sprite_test.go — the render-caching plan's Synth-tab
// TraceSprite adopters: the graph sprite's eviction (tab hide, locale change)
// and the per-param cards' pixel identity + render count. Mirrors
// eq_curve_sprite_test.go's pattern.

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

// TestSynthSpriteEvict_ReleasedOnTabHide mirrors
// TestTabSwitchReleasesTraceSprites (trace_sprite_eviction_test.go): leaving
// the Synth tab must release its graph sprite texture immediately via
// ReleaseSynthSprites (releaseHiddenTabSprites chokepoint), and returning
// lazily re-allocates on the next Draw.
func TestSynthSpriteEvict_ReleasedOnTabHide(t *testing.T) {
	g := driveScene(t, "crop_synth_tab_modular")
	dv := g.drum
	if dv.instEditorPreviewRect.Empty() {
		t.Skip("no desktop preview pane in this profile")
	}
	screen := ebiten.NewImage(1280, 720)
	g.Draw(screen)

	// residentAll re-reads live state from dv.synp().sprites each call — the
	// struct holds TraceSprite BY VALUE, so a snapshot copy taken before a
	// Release() would go stale (Release mutates the original via pointer
	// receiver on the struct field, not a local copy).
	// The right pane is the rendered-audio graph (synth_graph_pane.go), on
	// the root and drilled alike.
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

// TestSynthSprites_ReleasedOnLocaleChange guards against cached localized text
// (focus-graph caption, chip labels) going stale after a language switch: those
// sprites key on geometry/params, not locale, so OnLocaleChanged must release
// them (via the whole-DrumView chokepoint) or they blit the previous language
// until an unrelated edit. Regression for the whole-branch-review finding.
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

// ── per-param cards (Task 3) ────────────────────────────────────────────
//
// The Synth detail pane's ParamCard loop (drawSynthDetailPane, synth_panel_
// zone.go) previously re-rasterized every visible card's keycap chrome +
// text every frame regardless of whether the param moved. Each card now
// gets its own TraceSprite keyed by paramID (p.sprites.card(paramID)), so a
// steady frame blits N cached textures and an edit re-renders exactly the
// one card whose (name, value, gauge, state) key changed.

// newSynthTabForCardSprites builds a DrumView bound to the modular voice and
// lays out the Synth tab so dv.instEditorControls / dv.instEditorBindings and
// the selected section's card rects are populated (buildSynthTab +
// placeKnobsInSection both run inside layoutSynthTab's Layout call).
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

// cardKeyFor builds the exact sprite key the production loop in
// drawSynthDetailPane constructs for knob index kIdx, so both tests below
// exercise the real key shape rather than a hand-rolled stand-in.
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

// TestSynthParamCardSprite_MatchesDirect pins pixel identity between the
// cached sprite path and the RENDER_SAFE direct path for one card, mirroring
// the trace-sprite MatchesDirect tests above.
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

// TestSynthParamCards_OnlyEditedCardReRenders pins the per-card caching win:
// with >=2 visible cards, identical frames re-render nothing after the warm
// frame, and editing ONE card's value re-renders exactly that card's
// sprite — its siblings must stay cache hits (proving the cache is keyed
// per-paramID, not shared/invalidated as a block).
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

	// Warm frame drives the REAL production loop (drawSynthDetailPane,
	// synth_panel_zone.go) — not a hand-rolled re-implementation. This is
	// the wiring assertion: if the card loop is not routed through
	// p.sprites.card(id), p.sprites.cards never gets an entry and the
	// Fatalf below fires (RED before Step 3's wrap, GREEN after).
	p.drawSynthDetailPane(dst, instrument.ID(instID))

	sprites := make([]*TraceSprite, len(visible))
	for i, id := range paramIDs {
		s := p.sprites.cards[id]
		if s == nil || !s.resident() {
			t.Fatalf("drawSynthDetailPane did not sprite-wrap card %q (p.sprites.cards has no resident entry) — card loop not routed through p.sprites.card", id)
		}
		sprites[i] = s
	}

	// Deterministic clock, installed on every card sprite AFTER the warm
	// frame lazily created them, so the 33ms drag throttle never masks a
	// same-tick re-render in the assertions below. The warm frame's
	// lastRender was stamped with the REAL wall clock (the sprites didn't
	// exist yet to receive the override), which sits far in the future of
	// this Unix-epoch clock — a naive Sub() would go deeply negative and
	// read as "still within the throttle window". Invalidate + one more
	// render re-stamps lastRender under the injected clock so every
	// subsequent Sub() below is a real, comparable duration.
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

	// Flip one visible knob's value — try each until the derived gauge/
	// label actually changes (an enum's dot index might not move for
	// every delta), so the "one edited card" scenario below is real. Pure
	// key derivation, no draw — keeps the render-count trail below clean.
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
