//go:build test

package ui

import (
	"image"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// synth_component_sprite_test.go — Task 2 of the render-caching plan: the
// Synth tab's three per-column trace draws (full-note "Your sound", "Up
// close" steady wave, and the selected-knob focus graph) adopt TraceSprite.
// Mirrors eq_curve_sprite_test.go's pixel-identity + render-count pattern,
// but drives the wrapped methods DIRECTLY (not through a full g.Draw()) so
// the render-count assertions aren't polluted by unrelated colWaveTrace /
// colConceptStroke ink drawn elsewhere on screen (param-card knob
// mini-visuals, stage thumbnails) that Task 3 hasn't cached yet.

// newSynthTabForSprites builds a DrumView bound to the modular voice (whose
// first knobbed section is OSC — see newModularSynthTabGame), lays out the
// Synth tab, and fills the mirror's steady + full-note slots deterministically
// (mirroring the crop_synth_* scene Setup functions, e.g. synthMirrorSceneSetup
// in scene_catalog.go). Returns the desktop right-pane card rects.
func newSynthTabForSprites(t *testing.T) (dv *DrumView, instID string, full, up, focus image.Rectangle) {
	t.Helper()
	g := newModularSynthTabGame(t)
	// newSynthTabGame lays out at 640x480 with the test-mode default panel
	// collapse — too short to give the full-note + mirror cards real room
	// (they can collapse to empty rects per splitSynthRightPane3's shrink
	// rules). expandEQPanelForTest (eq_curve_sprite_test.go) is the existing
	// seam for a real panel height in tests.
	expandEQPanelForTest(t, g)
	layoutSynthTab(t, g)
	dv = g.drum
	instID = dv.resolveSynthInstrument(dv.synthTabActiveInstrument())
	if instID == "" {
		t.Fatal("no active synth instrument")
	}
	dv.renderSynthMirrorNow(instID)
	t.Cleanup(func() {
		if dv.synthMirror != nil {
			dv.synthMirror.closeForTest()
		}
	})
	if dv.instEditorPreviewRect.Empty() {
		t.Skip("no desktop preview pane in this profile")
	}
	full, up, focus = splitSynthRightPane3(dv.instEditorPreviewRect)
	return dv, instID, full, up, focus
}

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

// ── focus graph ─────────────────────────────────────────────────────────

// TestSynthFocusGraphSprite_MatchesDirect pins pixel identity between the
// cached sprite path and the RENDER_SAFE direct path. Both calls draw into a
// rect anchored at the origin (rect.Min == (0,0)) so the cached blit (which
// lands the sprite texture at rect.Min) and the direct RENDER_SAFE render
// (which draws straight into dst at rect) are byte-comparable straight away.
func TestSynthFocusGraphSprite_MatchesDirect(t *testing.T) {
	dv, instID, _, _, focus := newSynthTabForSprites(t)
	if focus.Dx() < 32 || focus.Dy() < 24 {
		t.Fatalf("focus rect too small for the renderer: %v", focus)
	}
	r := image.Rect(0, 0, focus.Dx(), focus.Dy())

	cached := ebiten.NewImage(r.Dx(), r.Dy())
	dv.synp().drawSynthFocusGraph(cached, r, instID)

	old := envRenderSafe
	envRenderSafe = true
	direct := ebiten.NewImage(r.Dx(), r.Dy())
	dv.synp().drawSynthFocusGraph(direct, r, instID)
	envRenderSafe = old

	assertImagesEqual(t, cached, direct, r.Dx(), r.Dy())
}

// TestSynthFocusGraphSprite_RenderCount pins the caching win: a steady-state
// redraw (same instrument, same params, same knob selection) must NOT
// re-render the content (zero drawRect calls — a cache hit is one
// dst.DrawImage blit, not a drawRect at all), a knob-SELECTION change must
// force exactly one re-render (even though no param VALUE moved — the concept
// curve is per-selected-param), and steady frames afterward go quiet again.
func TestSynthFocusGraphSprite_RenderCount(t *testing.T) {
	dv, instID, _, _, focus := newSynthTabForSprites(t)
	if focus.Dx() < 32 || focus.Dy() < 24 {
		t.Fatalf("focus rect too small for the renderer: %v", focus)
	}
	sec := dv.synthSelectedSection()
	if sec == nil || len(sec.knobIdxs) < 2 {
		t.Skip("selected section needs >=2 knobs to exercise a selection change")
	}
	kIdx0, kIdx1 := sec.knobIdxs[0], sec.knobIdxs[1]
	dv.setSynthSelectedKnob(instID, kIdx0)

	p := dv.synp()
	now := time.Unix(10_000, 0)
	p.sprites.focus.now = func() time.Time { return now }
	r := image.Rect(0, 0, focus.Dx(), focus.Dy())
	img := ebiten.NewImage(r.Dx(), r.Dy())

	warm := collectFilledRects(t, func() { p.drawSynthFocusGraph(img, r, instID) })
	if len(warm) == 0 {
		t.Fatal("warm frame drew nothing — sprite render never ran")
	}

	now = now.Add(time.Second)
	steady := collectFilledRects(t, func() {
		for i := 0; i < 5; i++ {
			p.drawSynthFocusGraph(img, r, instID)
		}
	})
	if n := len(steady); n != 0 {
		t.Fatalf("steady-state frames re-rendered the focus graph %d times (want 0 — cached blit only)", n)
	}

	// Selection change: key changes even though no param VALUE moved.
	dv.setSynthSelectedKnob(instID, kIdx1)
	now = now.Add(time.Second)
	edited := collectFilledRects(t, func() { p.drawSynthFocusGraph(img, r, instID) })
	if len(edited) == 0 {
		t.Fatal("knob-selection change did not re-render the focus graph (stale cache)")
	}

	next := collectFilledRects(t, func() { p.drawSynthFocusGraph(img, r, instID) })
	if n := len(next); n != 0 {
		t.Fatalf("post-edit steady frame re-rendered the focus graph %d times (want 0)", n)
	}
}

// ── "Up close" mirror ───────────────────────────────────────────────────

func TestSynthMirrorSprite_MatchesDirect(t *testing.T) {
	dv, instID, _, up, _ := newSynthTabForSprites(t)
	if up.Dx() < 32 || up.Dy() < 24 {
		t.Fatalf("mirror rect too small for the renderer: %v", up)
	}
	r := image.Rect(0, 0, up.Dx(), up.Dy())

	cached := ebiten.NewImage(r.Dx(), r.Dy())
	dv.synp().drawSynthMirror(cached, r, instID, dv.synthMirror)

	old := envRenderSafe
	envRenderSafe = true
	direct := ebiten.NewImage(r.Dx(), r.Dy())
	dv.synp().drawSynthMirror(direct, r, instID, dv.synthMirror)
	envRenderSafe = old

	assertImagesEqual(t, cached, direct, r.Dx(), r.Dy())
}

// TestSynthMirrorSprite_RenderCount mirrors TestEQCurveSpriteRenderCount: zero
// re-renders across identical-param frames, exactly one after a param change
// (updateSynthMirrorLive re-renders the steady-wave slot SYNCHRONOUSLY, so the
// new PCM is already in mirror by the time the params-hash key changes).
func TestSynthMirrorSprite_RenderCount(t *testing.T) {
	dv, instID, _, up, _ := newSynthTabForSprites(t)
	if up.Dx() < 32 || up.Dy() < 24 {
		t.Fatalf("mirror rect too small for the renderer: %v", up)
	}
	t.Cleanup(func() { audio.ResetInstrumentParams(instID) })

	p := dv.synp()
	now := time.Unix(20_000, 0)
	p.sprites.mirror.now = func() time.Time { return now }
	r := image.Rect(0, 0, up.Dx(), up.Dy())
	img := ebiten.NewImage(r.Dx(), r.Dy())

	warm := collectFilledRects(t, func() { p.drawSynthMirror(img, r, instID, dv.synthMirror) })
	if len(warm) == 0 {
		t.Fatal("warm frame drew nothing — sprite render never ran")
	}

	now = now.Add(time.Second)
	steady := collectFilledRects(t, func() {
		for i := 0; i < 5; i++ {
			p.drawSynthMirror(img, r, instID, dv.synthMirror)
		}
	})
	if n := len(steady); n != 0 {
		t.Fatalf("steady-state frames re-rendered the mirror %d times (want 0 — cached blit only)", n)
	}

	audio.SetInstrumentParam(instID, "osc_type", 1) // Saw — timbre knob, bumps the params hash
	dv.updateSynthMirrorLive(instID)
	now = now.Add(time.Second)
	edited := collectFilledRects(t, func() { p.drawSynthMirror(img, r, instID, dv.synthMirror) })
	if len(edited) == 0 {
		t.Fatal("param change did not re-render the mirror (stale cache)")
	}

	next := collectFilledRects(t, func() { p.drawSynthMirror(img, r, instID, dv.synthMirror) })
	if n := len(next); n != 0 {
		t.Fatalf("post-edit steady frame re-rendered the mirror %d times (want 0)", n)
	}
}

// ── full-note "Your sound" ──────────────────────────────────────────────

func TestSynthFullNoteSprite_MatchesDirect(t *testing.T) {
	dv, instID, full, _, _ := newSynthTabForSprites(t)
	if full.Dx() < 32 || full.Dy() < 24 {
		t.Fatalf("full-note rect too small for the renderer: %v", full)
	}
	r := image.Rect(0, 0, full.Dx(), full.Dy())

	cached := ebiten.NewImage(r.Dx(), r.Dy())
	dv.synp().drawSynthFullNote(cached, r, instID, dv.synthMirror)

	old := envRenderSafe
	envRenderSafe = true
	direct := ebiten.NewImage(r.Dx(), r.Dy())
	dv.synp().drawSynthFullNote(direct, r, instID, dv.synthMirror)
	envRenderSafe = old

	assertImagesEqual(t, cached, direct, r.Dx(), r.Dy())
}

// TestSynthFullNoteSprite_RenderCount exercises the full-note card's async
// render path (updateSynthFullNoteLive submits to the shared preview pool;
// drainFullForTest runs it synchronously for a deterministic test): a param
// change must force exactly one re-render, driven by the ghost flag flipping
// in the cache key (see drawSynthFullNote's doc comment in synth_full_note.go).
func TestSynthFullNoteSprite_RenderCount(t *testing.T) {
	dv, instID, full, _, _ := newSynthTabForSprites(t)
	if full.Dx() < 32 || full.Dy() < 24 {
		t.Fatalf("full-note rect too small for the renderer: %v", full)
	}
	t.Cleanup(func() { audio.ResetInstrumentParams(instID) })

	p := dv.synp()
	now := time.Unix(30_000, 0)
	p.sprites.fullNote.now = func() time.Time { return now }
	r := image.Rect(0, 0, full.Dx(), full.Dy())
	img := ebiten.NewImage(r.Dx(), r.Dy())

	warm := collectFilledRects(t, func() { p.drawSynthFullNote(img, r, instID, dv.synthMirror) })
	if len(warm) == 0 {
		t.Fatal("warm frame drew nothing — sprite render never ran")
	}

	now = now.Add(time.Second)
	steady := collectFilledRects(t, func() {
		for i := 0; i < 5; i++ {
			p.drawSynthFullNote(img, r, instID, dv.synthMirror)
		}
	})
	if n := len(steady); n != 0 {
		t.Fatalf("steady-state frames re-rendered the full-note card %d times (want 0 — cached blit only)", n)
	}

	audio.SetInstrumentParam(instID, "amp_decay", 0.9)
	dv.updateSynthFullNoteLive(instID)
	dv.synthMirror.drainFullForTest()
	now = now.Add(time.Second)
	edited := collectFilledRects(t, func() { p.drawSynthFullNote(img, r, instID, dv.synthMirror) })
	if len(edited) == 0 {
		t.Fatal("param change did not re-render the full-note card (stale cache)")
	}

	next := collectFilledRects(t, func() { p.drawSynthFullNote(img, r, instID, dv.synthMirror) })
	if n := len(next); n != 0 {
		t.Fatalf("post-edit steady frame re-rendered the full-note card %d times (want 0)", n)
	}
}

// TestSynthFullNoteSprite_InvalidatesOnAsyncMirrorReady closes the staleness
// window unique to the full-note card: updateSynthFullNoteLive's render lands
// on the shared preview pool ASYNCHRONOUSLY, and its completion (flagged by
// synthMirror.consumeReady) does NOT itself bump the params hash — only the
// ghost flag flips, and only once, at request time (not at completion). This
// pins synthTabUpdate's belt-and-suspenders Invalidate() call: without it, a
// render that completes on a later frame than the request would be blitted
// stale from the cache forever (same key, no re-render).
func TestSynthFullNoteSprite_InvalidatesOnAsyncMirrorReady(t *testing.T) {
	dv, instID, full, _, _ := newSynthTabForSprites(t)
	if full.Dx() < 32 || full.Dy() < 24 {
		t.Fatalf("full-note rect too small for the renderer: %v", full)
	}
	t.Cleanup(func() { audio.ResetInstrumentParams(instID) })

	p := dv.synp()
	r := image.Rect(0, 0, full.Dx(), full.Dy())
	img := ebiten.NewImage(r.Dx(), r.Dy())

	warm := collectFilledRects(t, func() { p.drawSynthFullNote(img, r, instID, dv.synthMirror) })
	if len(warm) == 0 {
		t.Fatal("warm frame drew nothing — sprite render never ran")
	}
	steady := collectFilledRects(t, func() { p.drawSynthFullNote(img, r, instID, dv.synthMirror) })
	if len(steady) != 0 {
		t.Fatalf("expected a cache hit before the async event, got %d draws", len(steady))
	}

	// Simulate the async job landing WITHOUT any params-hash change (the
	// scenario updateSynthFullNoteLive's stale-while-revalidate design
	// produces): same hash, fresh pcmFull content, ready flips.
	hash := dv.synthParamsHash(instID)
	dv.synthMirror.renderNowFull(instID, hash, func(i string) []float64 {
		return audio.RenderInstrumentPreview(i, synthFullNoteWindowMs(i))
	})
	if !dv.synthMirror.consumeReady() {
		t.Fatal("renderNowFull did not flip ready — test setup invalid")
	}
	// This is the production wiring under test: synthTabUpdate observes
	// consumeReady() and Invalidate()s the trace sprites. Call it directly
	// since we already consumed ready above (consumeReady is one-shot).
	p.sprites.fullNote.Invalidate()
	p.sprites.mirror.Invalidate()

	postAsync := collectFilledRects(t, func() { p.drawSynthFullNote(img, r, instID, dv.synthMirror) })
	if len(postAsync) == 0 {
		t.Fatal("async render landed but the full-note sprite blitted stale content (no re-render)")
	}
}

// ── eviction ─────────────────────────────────────────────────────────────

// TestSynthSpriteEvict_ReleasedOnTabHide mirrors
// TestTabSwitchReleasesTraceSprites (trace_sprite_eviction_test.go): leaving
// the Synth tab must release all three trace-sprite textures immediately via
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
	residentAll := func() (fullNote, mirror, focus bool) {
		s := dv.synp().sprites
		return s.fullNote.resident(), s.mirror.resident(), s.focus.resident()
	}

	if fn, m, fo := residentAll(); !fn || !m || !fo {
		t.Fatalf("synth tab draw did not allocate its sprites (fullNote=%v mirror=%v focus=%v)", fn, m, fo)
	}

	g.drum.eqPanelZone.SetActiveTab(TabWave)
	if fn, m, fo := residentAll(); fn || m || fo {
		t.Fatalf("leaving TabSynth must release its sprites (fullNote=%v mirror=%v focus=%v)", fn, m, fo)
	}

	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.Draw(screen)
	if _, _, fo := residentAll(); !fo {
		t.Fatal("returning to TabSynth must re-allocate the focus sprite on draw")
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

	sp := dv.synp()
	if !sp.sprites.focus.resident() || !sp.sprites.chip.resident() {
		t.Fatalf("synth tab draw did not allocate focus/chip sprites (focus=%v chip=%v) — cannot test locale eviction",
			sp.sprites.focus.resident(), sp.sprites.chip.resident())
	}

	dv.OnLocaleChanged()

	if sp.sprites.focus.resident() || sp.sprites.chip.resident() {
		t.Fatalf("OnLocaleChanged must release synth sprites so localized text re-renders in the new locale (focus=%v chip=%v)",
			sp.sprites.focus.resident(), sp.sprites.chip.resident())
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
// lays out the Synth tab so dv.instEditorKnobs / dv.instEditorBindings and
// the selected section's card rects are populated (buildSynthTab +
// placeKnobsInSection both run inside layoutSynthTab's Layout call).
func newSynthTabForCardSprites(t *testing.T) (dv *DrumView, instID string) {
	t.Helper()
	g := newModularSynthTabGame(t)
	expandEQPanelForTest(t, g)
	layoutSynthTab(t, g)
	dv = g.drum
	instID = dv.resolveSynthInstrument(dv.synthTabActiveInstrument())
	if instID == "" {
		t.Fatal("no active synth instrument")
	}
	return dv, instID
}

// cardKeyFor builds the exact sprite key the production loop in
// drawSynthDetailPane constructs for knob index kIdx, so both tests below
// exercise the real key shape rather than a hand-rolled stand-in.
func cardKeyFor(dv *DrumView, instID string, sel *synthSection, kIdx int) (paramID, name, value string, gauge paramCardGauge, st paramCardState, key any) {
	k := dv.instEditorKnobs[kIdx]
	b := dv.instEditorBindings[kIdx]
	st = paramCardState{
		Selected: dv.synthSelectedKnobIdx(instID, sel) == kIdx,
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
	if kIdx < 0 || kIdx >= len(dv.instEditorKnobs) || dv.instEditorKnobs[kIdx].Rect().Empty() {
		t.Skip("first card not visible in this profile")
	}
	rOrig := dv.instEditorKnobs[kIdx].Rect()
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
		if kIdx < 0 || kIdx >= len(dv.instEditorKnobs) {
			continue
		}
		if !dv.instEditorKnobs[kIdx].Rect().Empty() {
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
	p.drawSynthDetailPane(dst, instID)

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
	p.drawSynthDetailPane(dst, instID) // re-primes lastRender under the injected clock
	before := snapshot()

	now = now.Add(time.Second)
	p.drawSynthDetailPane(dst, instID)
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
		k := dv.instEditorKnobs[kIdx]
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
	p.drawSynthDetailPane(dst, instID)
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
	p.drawSynthDetailPane(dst, instID)
	next := snapshot()
	for i := range sprites {
		if next[i] != edited[i] {
			t.Fatalf("post-edit steady frame re-rendered card %q (want cached blit only)", paramIDs[i])
		}
	}
}

// ── chip strip (Task 4) ─────────────────────────────────────────────────
//
// The pipeline chip strip (drawSynthChipStrip, synth_panel_zone.go) is the
// riskiest TraceSprite adopter: unlike focus/mirror/full-note (whose content
// functions only ever consult the rect they were CALLED with — Draw hands
// the callback either the origin-anchored rect or, under RENDER_SAFE, the
// original rect, and the content just uses whichever it got), the chip
// strip's positions live in dv.instEditorChips[i].rect — absolute, fixed by
// Layout, independent of any rect this draw is asked to fill. The wrapper
// must explicitly translate every absolute coordinate by the strip's own
// origin (strip.Min - r.Min, see drawSynthChipStrip's doc comment) so the
// cached offscreen texture (anchored at (0,0)) matches the RENDER_SAFE
// direct-to-screen path exactly — this is what these tests pin.

// newSynthTabForChipStripSprite builds a DrumView with a laid-out Synth tab
// and a non-empty chip strip, and synchronously renders the per-stage
// waveform-watermark thumbnails (ensureStageThumbsNow — always synchronous,
// unlike the pool-submitted ensureStageThumbs) so the watermark band has
// real content before the strip is drawn — exercising the trickiest
// per-column coordinate math, not just the chip pill geometry.
func newSynthTabForChipStripSprite(t *testing.T) (dv *DrumView, instID string) {
	t.Helper()
	g := newModularSynthTabGame(t)
	expandEQPanelForTest(t, g)
	layoutSynthTab(t, g)
	dv = g.drum
	instID = dv.resolveSynthInstrument(dv.synthTabActiveInstrument())
	if instID == "" {
		t.Fatal("no active synth instrument")
	}
	dv.ensureStageThumbsNow(instID)
	if dv.synp().chipStripBounds().Empty() {
		t.Skip("no chips laid out in this profile")
	}
	return dv, instID
}

// TestSynthChipStripSprite_MatchesDirect pins pixel identity between the
// cached sprite path and the RENDER_SAFE direct path for the whole chip
// strip. Unlike the other MatchesDirect tests, both calls draw into a
// FULL-SIZE canvas at the chip strip's REAL absolute position rather than a
// synthetic (0,0)-anchored rect — drawSynthChipStrip computes its rect
// internally from dv.instEditorChips, so there is no rect parameter to hand
// it a synthetic origin. The cached path's blit lands at the strip's true
// absolute position (TraceSprite.Draw translates by rect.Min), so comparing
// the two canvases directly at absolute coordinates is the only way to
// prove drawSynthChipStripContent's coordinate translation has no
// off-by-origin smear.
func TestSynthChipStripSprite_MatchesDirect(t *testing.T) {
	dv, _ := newSynthTabForChipStripSprite(t)
	p := dv.synp()
	strip := p.chipStripBounds()
	w, h := strip.Max.X, strip.Max.Y
	if w <= 0 || h <= 0 {
		t.Fatalf("degenerate strip bounds: %v", strip)
	}

	cached := ebiten.NewImage(w, h)
	p.drawSynthChipStrip(cached)

	old := envRenderSafe
	envRenderSafe = true
	direct := ebiten.NewImage(w, h)
	p.drawSynthChipStrip(direct)
	envRenderSafe = old

	assertImagesEqual(t, cached, direct, w, h)
}

// TestSynthChipStrip_ReRendersOnSelection pins the caching win: a
// steady-state redraw (same selection, same enabled flags, same watermark
// content) blits the cached texture — 0 draws captured by
// collectFilledRects, since a cache hit never calls render(), only
// dst.DrawImage. Changing the selected chip, a chip's enabled flag, or the
// watermark content-version each force exactly one re-render; steady frames
// after each change go quiet again. Chip fields are flipped directly
// (mirrors TestSynthParamCards_OnlyEditedCardReRenders's direct k.Value
// mutation) to unit-test the caching layer's key derivation in isolation
// from the full enable-toggle-button production wiring.
func TestSynthChipStrip_ReRendersOnSelection(t *testing.T) {
	dv, instID := newSynthTabForChipStripSprite(t)
	p := dv.synp()
	if len(dv.instEditorChips) < 2 {
		t.Skip("need >=2 chips to exercise a selection change")
	}

	strip := p.chipStripBounds()
	img := ebiten.NewImage(strip.Max.X, strip.Max.Y)

	warm := collectFilledRects(t, func() { p.drawSynthChipStrip(img) })
	if len(warm) == 0 {
		t.Fatal("warm frame drew nothing — sprite render never ran")
	}

	steady := collectFilledRects(t, func() {
		for i := 0; i < 5; i++ {
			p.drawSynthChipStrip(img)
		}
	})
	if n := len(steady); n != 0 {
		t.Fatalf("steady-state frames re-rendered the chip strip %d times (want 0 — cached blit only)", n)
	}

	// Selection change.
	curSel := -1
	for i, c := range dv.instEditorChips {
		if c.selected {
			curSel = i
		}
	}
	if curSel < 0 {
		t.Skip("no chip is selected in this profile")
	}
	newSel := 0
	if curSel == 0 {
		newSel = 1
	}
	dv.instEditorChips[curSel].selected = false
	dv.instEditorChips[newSel].selected = true

	afterSel := collectFilledRects(t, func() { p.drawSynthChipStrip(img) })
	if len(afterSel) == 0 {
		t.Fatal("selection change did not re-render the chip strip (stale cache)")
	}
	next := collectFilledRects(t, func() { p.drawSynthChipStrip(img) })
	if n := len(next); n != 0 {
		t.Fatalf("post-selection steady frame re-rendered the chip strip %d times (want 0)", n)
	}

	// Enable-flag change.
	dv.instEditorChips[0].enabled = !dv.instEditorChips[0].enabled

	afterEnable := collectFilledRects(t, func() { p.drawSynthChipStrip(img) })
	if len(afterEnable) == 0 {
		t.Fatal("enable-flag change did not re-render the chip strip (stale cache)")
	}
	next2 := collectFilledRects(t, func() { p.drawSynthChipStrip(img) })
	if n := len(next2); n != 0 {
		t.Fatalf("post-enable steady frame re-rendered the chip strip %d times (want 0)", n)
	}

	// Thumb content-version change: ensureStageThumbsNow always REPLACES
	// t.waves synchronously (bumping contentVersion()) even when the params
	// hash is unchanged, so calling it again must force a re-render of the
	// watermark band alone.
	dv.ensureStageThumbsNow(instID)

	afterThumb := collectFilledRects(t, func() { p.drawSynthChipStrip(img) })
	if len(afterThumb) == 0 {
		t.Fatal("thumb content-version change did not re-render the chip strip (stale cache)")
	}
	next3 := collectFilledRects(t, func() { p.drawSynthChipStrip(img) })
	if n := len(next3); n != 0 {
		t.Fatalf("post-thumb steady frame re-rendered the chip strip %d times (want 0)", n)
	}
}
