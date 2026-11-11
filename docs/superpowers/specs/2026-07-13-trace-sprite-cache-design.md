# TraceSprite: generic cached-render surfaces for the audio panel (and beyond)

**Date:** 2026-07-13
**Status:** Approved (user, 2026-07-13) — approach A of the draw-hotpath investigation
**Owners:** internal/ui

## Problem — measured, not assumed

The audio panel's data surfaces re-render from scratch every frame as thousands of
tiny `drawRect` → `ebiten.Image.DrawImage` calls:

- Desktop CPU profile (30 s bench @120 BPM, EQ tab open): `Game.Draw` = 28.4 % of
  all process CPU. `EQPanelZone.Draw` = 3.26 s of `DrumView.Draw`'s 3.98 s.
  `drawEQCurve` alone = 2.24 s (13.8 % of ALL CPU) — the curve stroke + fill render
  as ~2 one-pixel-wide `drawRect` calls per plot column, per frame, plus dashed
  0 dB line, ±6 dB guides, and handles.
- WASM (webaudio_perf_e2e): **8,441 draw calls per frame**, `drawDrumMS` ≈ 23.3 ms.
- Ebiten v2.9.9 mechanics (verified in `internal/graphicscommand/command.go`):
  consecutive `DrawImage` calls with identical dst/src/blend/shader DO merge into
  one GPU command, so this is NOT a GL-call problem. The cost is the per-call
  Go-side overhead (vertex append, mipmap/atlas bookkeeping) × 8,441 — and on WASM
  that CPU runs on the single thread the audio scheduler shares. The Chain tab's
  existing `traceCache` comment states the consequence exactly: a Draw burst
  starves the sequencer; a cached blit "hands the sequencer goroutine a cheap
  frame".

The content being redrawn is almost always identical to the previous frame: EQ
curves change only on band edits; analyzer traces change at the analyzer tick
(~30 Hz); synth preview cards change only on param/section changes.

A second, related measured cost: the synth tab's drag path allocates ~92 MB of
`ebiten.Image.SubImage` handles across one test run (~1 M calls) — the
`TestSynthTab_LiveKnobEditFrameByteBudget` failure (351–912 KB/frame vs 224 KB
budget).

## Goal

Steady-state audio-panel frames become O(surfaces) instead of O(pixels): one
`DrawImage` blit per data surface plus live chrome. Target: WASM `drawDrumMS`
from ~23 ms to the low single digits with the panel open; draw calls per frame
from ~8,400 to < 50. Synth-tab drag frames return under their byte budget.

## Non-goals

- Grid pane and row rack (already texture-cached and cheap: ~1.4 ms).
- Text/glyph pipeline, buttons, portals (minor per profile).
- Converting rebuild internals to single-mesh `DrawTriangles` (a later,
  independent optimization inside rebuilds; approach B of the investigation).

## Design

### 1. The shared component: `TraceSprite`

One new file `internal/ui/trace_sprite.go` (+ `trace_sprite_test.go`). The name
avoids collision with the existing `gridRenderCache` and row "sprite cache".

```go
// TraceSprite memoizes a rect-sized rendering into an offscreen GPU texture,
// re-rendering only when the caller's version key changes (optionally rate-
// capped), and blitting one image per frame otherwise.
type TraceSprite struct {
    img         *ebiten.Image // offscreen target; nil until first render
    key         any           // comparable version key of the cached content
    keyOK       bool
    size        image.Point   // allocated size (realloc on change)
    lastRender  time.Time     // throttle bookkeeping
    minInterval time.Duration // 0 = re-render on every key change
    now         func() time.Time // injectable clock for tests (nil = time.Now)
}

// Draw ensures the cache reflects (key, rect size) — re-rendering via render
// when needed and allowed — then blits it into dst at rect.Min.
// render draws in RECT-LOCAL coordinates onto an image of rect.Size().
func (s *TraceSprite) Draw(dst *ebiten.Image, rect image.Rectangle, key any,
    render func(img *ebiten.Image, local image.Rectangle))

// Invalidate forces the next Draw to re-render (key mismatch shortcut).
func (s *TraceSprite) Invalidate()

// Release deallocates the texture immediately (ebiten.Image.Deallocate) and
// clears the key. The sprite remains usable — the next Draw re-allocates.
func (s *TraceSprite) Release()
```

Semantics (the careful parts):

- **Key**: any comparable value. Adopters fold EVERY render input into it — data
  version + theme/density revision + anything else the pixels depend on. The
  component compares with `!=` (adopters use small structs of comparables, e.g.
  `struct{ paramsHash uint64; themeRev, densityRev int; w, h int }`). Rect size
  is additionally checked by the component itself: size change ⇒ `Deallocate` +
  realloc + unconditional re-render (never throttled — a stale-sized blit would
  visibly smear).
- **Throttle** (`minInterval`, ~33 ms for drag-churn surfaces, 0 for pure
  event-driven ones): when the key differs but the interval hasn't elapsed, Draw
  blits the STALE image this frame. Because Draw runs every frame, the trailing
  edge is guaranteed: the first frame after the interval elapses re-renders the
  final state. No timer, no goroutine, no missed final frame.
- **Render callback contract**: receives a cleared (transparent) image of
  `rect.Size()` and draws in local coordinates (`local = rect.Sub(rect.Min)`).
  The component clears the image before each re-render (`img.Clear()`), so
  renders never accumulate.
- **Blit**: one `DrawImage` with a translate GeoM; no color scale, no filter
  (1:1 pixels) — pixel-identical to direct drawing by construction.

### 2. Lifecycle, clearing, and GC/flush discipline

This is where Go + ebiten patterns matter:

- **Explicit deallocation over GC finalizers.** Ebiten images are eventually
  reclaimed by the Go GC via finalizers, but that is nondeterministic and (per
  today's measurements) the game can run 20+ s between GCs — a hidden tab's
  texture would linger. Ebiten v2.7+ provides `(*Image).Deallocate()` which
  returns the texture immediately and keeps the handle reusable. `Release()`
  uses it; eviction never relies on the GC. (Go GC remains the backstop if an
  owner forgets.)
- **Ownership + eviction chokepoint.** Each Zone owns its sprites as struct
  fields. Eviction hooks into the existing single visibility chokepoint: the
  `DrumViewTree` visibility predicate already gates Draw + HitAreas per zone
  (`RegisterZoneVisible`); zones release their sprites when their tab
  deactivates via the existing `EnsureAnalyzersForTab`/`SetActiveTab` dispatcher
  path (the same chokepoint that manages analyzer enablement — no fourth
  tab-switch path is introduced). Only the ACTIVE tab's sprites are resident:
  ~1280×400 RGBA ≈ 2 MB × 2–3 surfaces ≈ ≤ 6 MB GPU, desktop and WASM.
- **Render-target isolation.** The first render INTO an `ebiten.Image` makes it
  an unmanaged/isolated texture in ebiten's atlas system — offscreen targets do
  not fragment the sprite atlases. Allocations are rare (first show + resize),
  so `NewImage` cost is off the steady-state path. Resize storms (splitter
  drags) are naturally bounded because Layout emits at most one size per frame
  and the component reallocates only on actual size change.
- **`RENDER_SAFE=1` bypass.** When the env flag (already used by the pixel
  tests) is set, `TraceSprite.Draw` calls `render` directly against `dst`
  (translated), skipping the cache entirely — the `drawRect` interceptor tests
  and `audio_panel_render_pixels_test.go` keep seeing every primitive.
- **No per-frame allocations.** Steady-state Draw allocates nothing (key compare
  + one DrawImage). The discipline budgets in
  `eq_panel_draw_alloc_discipline_test.go` ratchet DOWN accordingly.

### 3. Adopters (rollout order)

1. **EQ tab** — `drawEQCurve` (curve + fill + 0 dB dashes + guides; handles stay
   live per-frame on top) and `drawSpectrumBars`. Keys: curve = EQ gains/HPF/LPF
   hash + rect + theme rev, `minInterval=33ms` (handle drags); spectrum =
   analyzer snapshot pointer identity (the Chain tab precedent), `minInterval=0`
   (the tick IS the rate).
2. **Wave tab** — `drawAnalyzerWaveform` trace (cursor crosshair stays live).
3. **Levels tab** — per-channel strips + aggregate column (latch text stays
   live; it changes per tick anyway — measure and split live vs cached there).
4. **Synth tab right pane + stage cards** — preview cards and mirror keyed by
   (instrument, paramsHash, section, rect), `minInterval=33ms` — this closes the
   drag-frame byte-budget failure. The rebuild path also caches its `SubImage`
   handles (a `map[image.Rectangle]*ebiten.Image` per parent image, cleared with
   the sprite) to kill the 92 MB SubImage churn *within* rebuilds.
5. **Chain tab** — migrate the bespoke `traceCache` onto `TraceSprite` (behavior
   already matches; deletes the one-off).

Each adopter keeps its existing render function body — the code moves inside the
`render` callback with rect-local coordinates (they already compute from a rect
parameter; the call site passes the local rect).

### 4. Reusability beyond the panel

The component is deliberately Zone-agnostic: any widget whose pixels are a pure
function of identifiable inputs (component state, parent-provided data, layout
rect) can adopt it by (a) folding those inputs into a comparable key and
(b) rendering in local coordinates. Documented in the type comment with the
adoption checklist. Candidates NOT converted now (cheap today): timeline ribbon,
sticky-bar chrome.

### 5. Testing (the shared infra is tested exhaustively)

Unit tests (`trace_sprite_test.go`, injectable clock, counting render func):

- Render-count semantics: same key ⇒ 0 renders; key change ⇒ exactly 1; N rapid
  key changes under `minInterval` ⇒ renders ≤ ceil(elapsed/interval)+1 and the
  FINAL key is always rendered once churn stops (trailing edge).
- Size change ⇒ Deallocate + realloc + immediate render even mid-throttle.
- `Invalidate` ⇒ next Draw renders once; `Release` ⇒ texture deallocated
  (observable via ebitenstub — add a `Deallocate` no-op/counter to the stub if
  absent) and next Draw re-allocates + renders.
- Clear-before-render: two renders with disjoint content don't accumulate
  (pixel read via ebitenstub `At`).
- Zero/degenerate rects are safe no-ops.
- `RENDER_SAFE` bypass renders directly to dst every frame (interceptor sees
  primitives).
- Nil-clock/default construction works (`var s TraceSprite` usable zero-value).

Integration/regression tests:

- **Pixel identity per adopter** (the `TestChainTraceCacheMatchesDirect`
  pattern): cached blit byte-identical to direct render for EQ curve, spectrum,
  wave, levels, synth cards, via ebitenstub pixel reads.
- `audio_panel_render_pixels_test.go` (RENDER_SAFE path) still green untouched.
- Draw-call regression: functional test drives real `Game.Update`/`Draw` with
  the EQ tab open at steady state and asserts the per-frame primitive count via
  the existing drawRect/DrawImage interceptor drops below a pinned budget
  (ratchet, with the old count cited).
- `eq_panel_draw_alloc_discipline_test.go` budgets ratcheted down.
- **Acceptance:** `TestSynthTab_LiveKnobEditFrameByteBudget` returns green
  (drag ≤ 224 KB/frame) after step 4 — an existing failing test becomes the
  proof.
- Chain migration guarded by the existing `TestChainTraceCacheMatchesDirect`.

Perf verification (post-implementation): repeat the desktop pprof bench (EQ tab)
and `webaudio_perf_e2e` draw stats; record before/after in the completion notes.

## Error handling

- A panicking render callback must not wedge the sprite: the component does not
  recover (matches repo style — draw panics are bugs), but `keyOK` is set only
  AFTER a successful render, so a retried frame re-renders rather than blitting
  a half-drawn cache.
- Ebiten context loss is handled by ebiten's restorable layer (offscreens are
  restored or cleared); worst case one stale/blank frame followed by a normal
  re-render on the next key change — acceptable, documented.

## Risks

- Missed key input ⇒ stale pixels. Mitigated by the adoption checklist (fold
  theme/density/rect + every data input) and per-adopter pixel-identity tests
  that exercise input changes.
- Hidden-tab memory if an owner skips Release — bounded by Go GC backstop and
  the single tab-switch chokepoint doing the releasing centrally.
