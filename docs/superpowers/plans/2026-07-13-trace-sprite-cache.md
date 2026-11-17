# TraceSprite Cached-Render Surfaces Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the audio panel's per-frame thousands-of-DrawImage trace rendering with a generic cached-render component (`TraceSprite`) so steady-state frames blit one texture per data surface.

**Architecture:** A small reusable `TraceSprite` type (offscreen `ebiten.Image` + comparable version key + rect-local render callback + optional 30 Hz throttle + explicit `Deallocate` lifecycle) is adopted by the EQ curve, Spectrum, Wave, Levels, and Synth-tab surfaces; the Chain tab's bespoke `traceCache` migrates onto it. Eviction hooks into the existing tab-switch chokepoint. Spec: `docs/superpowers/specs/2026-07-13-trace-sprite-cache-design.md`.

**Tech Stack:** Go (bundled `.tools/go/bin/go`, Go 1.26.5), ebiten v2.9.9 (prod) / `internal/ebitestub` (test builds), fast test path `-tags test -modfile=go.test.mod`.

## Global Constraints

- **NEVER `git add` / `git commit` / stash / revert — leave all changes uncommitted.** (User convention; overrides this skill's default commit steps.)
- Fast tests: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod <pkg>`; ui suite needs `-timeout 1200s`.
- Format only files you touched: `../../.tools/go/bin/gofmt -w <files>`.
- Known pre-existing failures — NOT yours to fix, do not "fix" by weakening: `TestFocusGraph_RespondsToEveryKnob` (flaky), and `TestSynthTab_LiveKnobEditFrameByteBudget` which MUST TURN GREEN by Task 7 (it is the acceptance test).
- `RENDER_SAFE=1` (var `envRenderSafe`, game_draw_grid_pane.go:16) must bypass all caching — pixel tests depend on seeing raw primitives.
- Steady-state `TraceSprite.Draw` must not allocate (discipline budgets in `eq_panel_draw_alloc_discipline_test.go` are ratchets).
- The render callback contract: `render(img, r)` draws the content of area `r` onto `img`. Cached path passes the offscreen image and a rect at origin; RENDER_SAFE path passes the real dst and the original rect. Callbacks therefore must draw strictly relative to the rect they are handed (the existing helpers already take a rect parameter).
- `Zone` draw dispatch discipline: do not add new direct draw sites; all changes stay inside the existing zone Draw methods.

---

### Task 1: ebitenstub — correct 1:1 blits and Deallocate-safe reads

The stub's `DrawImage` copies only `src.pix[0]` across the destination (built for the 1×1 stretched-pixel `drawRectBlit` pattern). A cached TraceSprite blit is an UNSCALED multi-pixel draw — the stub must copy per-pixel for that case or every pixel-identity test in later tasks is meaningless. Also `At` must be safe after `Deallocate` (pix == nil).

**Files:**
- Modify: `src/go/internal/ebitestub/ebiten/ebiten.go:33-72` (DrawImage), `:110` (At)
- Test: `src/go/internal/ebitestub/ebiten/` has no test files today — the behavior is pinned from the ui package in Task 2's tests; this task's gate is the full existing stub-dependent suite.

**Interfaces:**
- Consumes: existing stub `Image{w,h int; pix []color.RGBA}`.
- Produces: `DrawImage` copies src per-pixel when `scaleX==1 && scaleY==1 && len(src.pix)==src.w*src.h`; scaled draws keep the legacy first-pixel fill. `At` returns `color.RGBA{}` when `pix == nil` or out of bounds.

- [ ] **Step 1: Patch DrawImage.** In `func (i *Image) DrawImage`, after computing `tx, ty` and before the fill loop, add the unscaled-copy branch:

```go
	// Unscaled multi-pixel blit (TraceSprite cached-surface draws): copy
	// per-pixel so pixel-identity tests see real content. Scaled draws keep
	// the legacy first-pixel fill (the 1x1 stretched drawRectBlit pattern).
	if scaleX == 1 && scaleY == 1 && len(src.pix) == src.w*src.h && src.w > 1 {
		for y := 0; y < src.h; y++ {
			dstY := ty + y
			if dstY < 0 || dstY >= i.h {
				continue
			}
			for x := 0; x < src.w; x++ {
				dstX := tx + x
				if dstX < 0 || dstX >= i.w {
					continue
				}
				c := src.pix[y*src.w+x]
				if c.A == 0 {
					continue // preserve dst where the cache is transparent
				}
				i.pix[dstY*i.w+dstX] = c
			}
		}
		return
	}
```

The `c.A == 0` skip approximates source-over blending for the transparent-cleared cache background; cached content pixels are opaque in practice (token colors carry alpha ≥ AlphaFaint over an opaque panel — the identity tests in later tasks compare cached-blit against direct-draw on the SAME opaque background, so approximation differences cancel).

- [ ] **Step 2: Guard At.** Make `At` return `color.RGBA{}` when `i == nil || i.pix == nil` or x/y out of `[0,w)×[0,h)` (inspect the existing body first; add only the missing guards).

- [ ] **Step 3: Gate — full stub-dependent fast suite.**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -timeout 1500s ./...`
Expected: identical failure set to before this task (only the two known pre-existing failures). If any OTHER test changed state, the stub change broke an assumption — investigate before proceeding (the likely suspect: a test relying on first-pixel smear for an unscaled multi-pixel draw; fix the branch condition, not the test).

- [ ] **Step 4: gofmt touched file.** Leave uncommitted.

---

### Task 2: TraceSprite core + exhaustive unit tests

**Files:**
- Create: `src/go/internal/ui/trace_sprite.go`
- Create: `src/go/internal/ui/trace_sprite_test.go`

**Interfaces:**
- Consumes: `envRenderSafe` (game_draw_grid_pane.go:16), ebiten `Image.Clear/Deallocate/DrawImage`.
- Produces (used by Tasks 3-8): type `TraceSprite` with fields `minInterval time.Duration`, `now func() time.Time`; methods `Draw(dst *ebiten.Image, rect image.Rectangle, key any, render func(*ebiten.Image, image.Rectangle))`, `Invalidate()`, `Release()`. Const `traceSpriteDragInterval = 33 * time.Millisecond`.

- [ ] **Step 1: Write the failing tests** (`trace_sprite_test.go`):

```go
package ui

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

// countingRender returns a render func that stamps a solid color and counts
// invocations, recording the key-agnostic call order.
func countingRender(calls *int, col color.RGBA) func(img *ebiten.Image, r image.Rectangle) {
	return func(img *ebiten.Image, r image.Rectangle) {
		*calls++
		drawRect(img, r, col, true)
	}
}

func TestTraceSpriteRendersOncePerKey(t *testing.T) {
	var s TraceSprite
	dst := ebiten.NewImage(64, 32)
	rect := image.Rect(8, 4, 40, 28)
	calls := 0
	render := countingRender(&calls, color.RGBA{10, 20, 30, 255})

	for i := 0; i < 5; i++ {
		s.Draw(dst, rect, "k1", render)
	}
	if calls != 1 {
		t.Fatalf("same key: want exactly 1 render, got %d", calls)
	}
	s.Draw(dst, rect, "k2", render)
	if calls != 2 {
		t.Fatalf("key change: want 2nd render, got %d", calls)
	}
	// Blit reproduces content at the rect offset.
	if got := dst.At(9, 5); got != (color.RGBA{10, 20, 30, 255}) {
		t.Fatalf("blitted pixel mismatch: %v", got)
	}
	// Outside the rect stays untouched.
	if got := dst.At(1, 1); got != (color.RGBA{}) {
		t.Fatalf("pixel outside rect written: %v", got)
	}
}

func TestTraceSpriteThrottleTrailingEdge(t *testing.T) {
	now := time.Unix(1000, 0)
	s := TraceSprite{minInterval: 33 * time.Millisecond, now: func() time.Time { return now }}
	dst := ebiten.NewImage(32, 32)
	rect := image.Rect(0, 0, 16, 16)
	calls := 0
	render := countingRender(&calls, color.RGBA{255, 0, 0, 255})

	// Churn: a new key every simulated frame (16ms), 6 frames.
	for i := 0; i < 6; i++ {
		s.Draw(dst, rect, i, render)
		now = now.Add(16 * time.Millisecond)
	}
	// 96ms of churn at 33ms interval: first render fires immediately (keyOK
	// false), then at most every 33ms — expect 3 renders (t=0, ~33, ~66),
	// never 6.
	if calls != 3 {
		t.Fatalf("throttled churn: want 3 renders, got %d", calls)
	}
	// Trailing edge: churn stopped at key=5, which was NOT rendered (last
	// render saw key 4 at t=64... whichever — the guarantee is the next
	// Draw after the interval renders the final key exactly once).
	now = now.Add(40 * time.Millisecond)
	s.Draw(dst, rect, 5, render)
	if calls != 4 {
		t.Fatalf("trailing edge: want final-state render, got %d calls", calls)
	}
	s.Draw(dst, rect, 5, render)
	if calls != 4 {
		t.Fatalf("settled: want no further renders, got %d", calls)
	}
}

func TestTraceSpriteResizeForcesRenderEvenMidThrottle(t *testing.T) {
	now := time.Unix(2000, 0)
	s := TraceSprite{minInterval: time.Hour, now: func() time.Time { return now }}
	dst := ebiten.NewImage(64, 64)
	calls := 0
	render := countingRender(&calls, color.RGBA{0, 255, 0, 255})

	s.Draw(dst, image.Rect(0, 0, 16, 16), "k", render)
	s.Draw(dst, image.Rect(0, 0, 24, 16), "k", render) // size change, same key
	if calls != 2 {
		t.Fatalf("resize must re-render immediately (throttle bypassed), got %d calls", calls)
	}
	// A stale-size blit must never happen: content covers the NEW rect.
	if got := dst.At(20, 8); got != (color.RGBA{0, 255, 0, 255}) {
		t.Fatalf("resized content missing at new width: %v", got)
	}
}

func TestTraceSpriteInvalidateAndRelease(t *testing.T) {
	var s TraceSprite
	dst := ebiten.NewImage(32, 32)
	rect := image.Rect(0, 0, 8, 8)
	calls := 0
	render := countingRender(&calls, color.RGBA{1, 2, 3, 255})

	s.Draw(dst, rect, "k", render)
	s.Invalidate()
	s.Draw(dst, rect, "k", render) // same key, but invalidated
	if calls != 2 {
		t.Fatalf("Invalidate: want re-render on same key, got %d", calls)
	}
	s.Release()
	if s.img != nil {
		t.Fatal("Release must drop the texture handle")
	}
	s.Draw(dst, rect, "k", render) // must re-allocate and render, not panic
	if calls != 3 {
		t.Fatalf("post-Release Draw: want re-render, got %d", calls)
	}
}

func TestTraceSpriteClearsBetweenRenders(t *testing.T) {
	var s TraceSprite
	dst := ebiten.NewImage(64, 32)
	rect := image.Rect(0, 0, 32, 16)
	// First render fills the LEFT half; second render fills the RIGHT half.
	left := func(img *ebiten.Image, r image.Rectangle) {
		drawRect(img, image.Rect(r.Min.X, r.Min.Y, r.Min.X+16, r.Max.Y), color.RGBA{255, 0, 0, 255}, true)
	}
	right := func(img *ebiten.Image, r image.Rectangle) {
		drawRect(img, image.Rect(r.Min.X+16, r.Min.Y, r.Max.X, r.Max.Y), color.RGBA{0, 0, 255, 255}, true)
	}
	s.Draw(dst, rect, 1, left)
	dst.Clear()
	s.Draw(dst, rect, 2, right)
	// The left half must NOT retain the first render (cache cleared).
	if got := dst.At(4, 4); got != (color.RGBA{}) {
		t.Fatalf("stale content accumulated across renders: %v", got)
	}
	if got := dst.At(20, 4); got != (color.RGBA{0, 0, 255, 255}) {
		t.Fatalf("second render content missing: %v", got)
	}
}

func TestTraceSpriteDegenerateRectsAndNils(t *testing.T) {
	var s TraceSprite
	dst := ebiten.NewImage(8, 8)
	calls := 0
	render := countingRender(&calls, color.RGBA{9, 9, 9, 255})
	s.Draw(dst, image.Rect(0, 0, 0, 10), "k", render) // zero width
	s.Draw(dst, image.Rect(0, 0, 10, 0), "k", render) // zero height
	s.Draw(nil, image.Rect(0, 0, 4, 4), "k", render)  // nil dst
	s.Draw(dst, image.Rect(0, 0, 4, 4), "k", nil)     // nil render
	if calls != 0 {
		t.Fatalf("degenerate inputs must be no-ops, got %d renders", calls)
	}
}

func TestTraceSpriteRenderSafeBypass(t *testing.T) {
	old := envRenderSafe
	envRenderSafe = true
	t.Cleanup(func() { envRenderSafe = old })

	var s TraceSprite
	dst := ebiten.NewImage(32, 32)
	rect := image.Rect(4, 4, 20, 20)
	calls := 0
	var gotRect image.Rectangle
	render := func(img *ebiten.Image, r image.Rectangle) {
		calls++
		gotRect = r
		if img != dst {
			t.Fatal("RENDER_SAFE bypass must render directly onto dst")
		}
	}
	for i := 0; i < 3; i++ {
		s.Draw(dst, rect, "same-key", render)
	}
	if calls != 3 {
		t.Fatalf("bypass renders every frame, got %d", calls)
	}
	if gotRect != rect {
		t.Fatalf("bypass must pass the ORIGINAL rect, got %v", gotRect)
	}
	if s.img != nil {
		t.Fatal("bypass must not allocate a cache texture")
	}
}

func TestTraceSpriteSteadyStateDoesNotAllocate(t *testing.T) {
	var s TraceSprite
	dst := ebiten.NewImage(64, 64)
	rect := image.Rect(0, 0, 48, 48)
	render := func(img *ebiten.Image, r image.Rectangle) {
		drawRect(img, r, color.RGBA{7, 7, 7, 255}, true)
	}
	s.Draw(dst, rect, "k", render) // warm
	avg := testing.AllocsPerRun(200, func() {
		s.Draw(dst, rect, "k", render)
	})
	// One interface boxing of the key literal is tolerated ("k" is constant
	// so it should be zero); the blit itself must be alloc-free.
	if avg > 1 {
		t.Fatalf("steady-state Draw allocates %.1f allocs/op (want <=1)", avg)
	}
}
```

- [ ] **Step 2: Run to verify failure.**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestTraceSprite' ./internal/ui/`
Expected: compile FAIL — `TraceSprite` undefined.

- [ ] **Step 3: Implement `trace_sprite.go`:**

```go
package ui

import (
	"image"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

// traceSpriteDragInterval is the re-render cap for surfaces whose key churns
// continuously during a drag (EQ handle, synth knob): ~30 Hz, user-approved
// staleness (2026-07-13 design). Event-driven surfaces use 0 (every change).
const traceSpriteDragInterval = 33 * time.Millisecond

// TraceSprite memoizes a rect-sized rendering into an offscreen GPU texture,
// re-rendering only when the caller's version key changes (optionally rate-
// capped) and blitting ONE image per frame otherwise.
//
// Why: the audio panel's data surfaces rendered as thousands of one-pixel
// drawRect->DrawImage calls per frame (drawEQCurve alone measured 13.8% of
// process CPU; 8,441 draw calls/frame on WASM). Ebiten merges consecutive
// same-source draws into one GPU command, so the cost is per-call Go-side
// overhead — on WASM it runs on the single thread the audio scheduler
// shares. A cached blit hands that thread back.
//
// Adoption checklist (any widget whose pixels are a pure function of
// identifiable inputs can adopt this):
//  1. Fold EVERY render input into `key` — data version/hash, density,
//     language, selection state... anything the pixels depend on. The key
//     must be a comparable value (small struct of comparables). Rect SIZE is
//     handled internally; rect position is not part of the content.
//  2. Render in rect-relative coordinates: the callback receives the target
//     image and the rect to draw within (at origin when cached; the original
//     rect under RENDER_SAFE bypass). Existing rect-parameter helpers work
//     unchanged.
//  3. Keep live per-frame elements (cursors, drag handles) OUT of the
//     callback — draw them on top after Draw returns.
//  4. Release() when the owning surface hides (tab switch) — textures are
//     reclaimed via ebiten.Image.Deallocate immediately instead of waiting
//     for a Go GC cycle (the game can run 20+ s between GCs).
type TraceSprite struct {
	img   *ebiten.Image
	key   any
	keyOK bool
	size  image.Point

	lastRender time.Time
	// minInterval rate-caps re-renders while the key churns (0 = re-render
	// on every change). The trailing edge is guaranteed without timers:
	// Draw runs every frame, so the first frame after the interval elapses
	// re-renders the final key.
	minInterval time.Duration
	// now is injectable for deterministic throttle tests; nil = time.Now.
	now func() time.Time
}

func (s *TraceSprite) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

// Draw ensures the cached texture reflects (key, rect size) — re-rendering
// via render when needed and allowed — then blits it into dst at rect.Min.
func (s *TraceSprite) Draw(dst *ebiten.Image, rect image.Rectangle, key any, render func(img *ebiten.Image, r image.Rectangle)) {
	if dst == nil || render == nil || rect.Dx() <= 0 || rect.Dy() <= 0 {
		return
	}
	if envRenderSafe {
		// Pixel-test path: every primitive must reach the interceptor-
		// instrumented drawRect against the real destination.
		render(dst, rect)
		return
	}
	if s.img == nil || s.size != rect.Size() {
		if s.img != nil {
			s.img.Deallocate()
		}
		s.img = ebiten.NewImage(rect.Dx(), rect.Dy())
		s.size = rect.Size()
		// A stale-size blit would smear: force the render below regardless
		// of the throttle by dropping key validity.
		s.keyOK = false
	}
	if !s.keyOK || key != s.key {
		throttled := s.keyOK && s.minInterval > 0 && s.clock().Sub(s.lastRender) < s.minInterval
		if !throttled {
			s.img.Clear()
			// keyOK stays false until render returns so a panicking render
			// re-renders next frame instead of blitting a half-drawn cache.
			s.keyOK = false
			render(s.img, image.Rect(0, 0, rect.Dx(), rect.Dy()))
			s.key = key
			s.keyOK = true
			s.lastRender = s.clock()
		}
	}
	drawRectOp.GeoM.Reset()
	drawRectOp.GeoM.Translate(float64(rect.Min.X), float64(rect.Min.Y))
	dst.DrawImage(s.img, &drawRectOp)
}

// Invalidate forces the next Draw to re-render (bypasses key equality, not
// the throttle-on-churn — a forced render is immediate because keyOK gates
// the throttle).
func (s *TraceSprite) Invalidate() { s.keyOK = false }

// Release deallocates the texture immediately and clears the cache state.
// The sprite stays usable: the next Draw re-allocates. Call on tab hide.
func (s *TraceSprite) Release() {
	if s.img != nil {
		s.img.Deallocate()
		s.img = nil
	}
	s.size = image.Point{}
	s.keyOK = false
}
```

NOTE: `drawRectOp` is the existing package-scratch `ebiten.DrawImageOptions` (drawing.go:20) — reusing it keeps the blit alloc-free; Draw runs on the UI goroutine only (same discipline as every draw helper). If its `GeoM` lacks `Reset()` in the stub, add `func (g *GeoM) Reset() { *g = GeoM{} }` to `internal/ebitestub/ebiten/geom.go` (the real ebiten has it).

- [ ] **Step 4: Run the unit tests.**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestTraceSprite' -count=1 ./internal/ui/`
Expected: PASS (all 8).

- [ ] **Step 5: Real-build compile check** (trace_sprite.go has no build tag — it must compile against real ebiten too):

Run: `cd src/go && CGO_ENABLED=1 ../../.tools/go/bin/go build ./internal/ui/ && GOOS=js GOARCH=wasm ../../.tools/go/bin/go build ./internal/ui/`
Expected: both succeed.

- [ ] **Step 6: gofmt touched files.** Leave uncommitted.

---

### Task 3: EQ curve adoption (the template adopter) + identity & draw-call-ratchet tests

**Files:**
- Modify: `src/go/internal/ui/eq_panel_zone.go:2616-2694` (drawEQCurve; the handles block from ~2696 stays live), EQPanelZone struct (add sprite field near `curveCache`), `rebuildCurveCache` (add revision bump).
- Create: `src/go/internal/ui/eq_curve_sprite_test.go`

**Interfaces:**
- Consumes: `TraceSprite` (Task 2).
- Produces: `EQPanelZone.curveSprite TraceSprite`, `EQPanelZone.curveRev int` (bumped by `rebuildCurveCache`), method `renderEQCurveContent(img *ebiten.Image, r image.Rectangle)` (the extracted body), key type `eqCurveKey struct{ rev int; selBand int; density Density; lang string; w, h int }`.

- [ ] **Step 1: Extract the content renderer.** Move the body of `drawEQCurve` from the "0 dB reference line" comment (line ~2637) through the end of the curve loop (line ~2694) — everything EXCEPT the cache-ensure preamble and the "Band handles" block — into a new method with the identical code:

```go
// renderEQCurveContent draws the static curve content (0 dB dashes, ±6 dB
// guides + labels, curve fill + stroke) into r on img. Handles are LIVE
// elements drawn by drawEQCurve after the sprite blit. Coordinates are
// r-relative (TraceSprite passes an origin rect when cached).
func (z *EQPanelZone) renderEQCurveContent(img *ebiten.Image, r image.Rectangle) {
	curveMinX, curveMaxX := z.eqCurveSpanX(r)
	// ... [the moved body, verbatim: zeroY dashes loop, guides loop with
	//      labels, curve fill+stroke loop — unchanged except it now uses the
	//      r parameter it already threaded through every call] ...
}
```

- [ ] **Step 2: Add the sprite + revision.** In the `EQPanelZone` struct next to `curveCache`:

```go
	// curveSprite caches the rendered curve content (fill/stroke/guides) —
	// ~2 DrawImage per plot column per frame before this (13.8% of process
	// CPU); one blit per frame after. Handles stay live. See trace_sprite.go.
	curveSprite TraceSprite
	// curveRev increments on every rebuildCurveCache so the sprite key
	// tracks actual curve-data changes (band edits, HPF/LPF).
	curveRev int
```

At the end of `rebuildCurveCache` add `z.curveRev++`. In `NewEQPanelZone` (or lazily — zero value works) set `z.curveSprite.minInterval = traceSpriteDragInterval` (handle drags churn the rev every frame).

- [ ] **Step 3: Rewrite `drawEQCurve` as orchestration:**

```go
func (z *EQPanelZone) drawEQCurve(dst *ebiten.Image) {
	r := z.eqPlotRect()
	if r.Dx() < 16 || r.Dy() < 16 {
		return
	}
	if z.curveDirty || len(z.curveCache) == 0 {
		z.rebuildCurveCache()
	}
	if len(z.curveCache) == 0 {
		return
	}
	key := eqCurveKey{
		rev:     z.curveRev,
		selBand: z.selectedBandForKey(), // whatever field feeds the selected-band emphasis IF it affects non-handle pixels; if only handles differ, omit
		density: Profile().Density(),
		lang:    i18n.Lang(),
		w:       r.Dx(),
		h:       r.Dy(),
	}
	z.curveSprite.Draw(dst, r, key, z.renderEQCurveContent)

	// Band handles stay live: they track the cursor mid-drag at full frame
	// rate. [existing handles block, verbatim, unchanged, drawing to dst]
	...
}
```

(Implementer: check the actual selected-band and i18n accessors at the call site — `i18n.Lang()` name per `internal/i18n`; if the guide labels aren't localized and no other pixel depends on language, drop `lang` from the key and note it. Keys are cheap; over-keying is safe, under-keying is a stale-pixel bug.)

- [ ] **Step 4: Write the identity + behavior tests** (`eq_curve_sprite_test.go`): a functional test that builds the real Game (see `testutil_test.go` patterns), opens the EQ tab, then:

```go
// TestEQCurveSpriteMatchesDirect renders one frame with the sprite path and
// one with RENDER_SAFE bypass into equal-size images and asserts the curve
// plot region is pixel-identical (the TestChainTraceCacheMatchesDirect
// pattern). Exercises: initial render, a band-gain edit (key change), and a
// plot resize.
func TestEQCurveSpriteMatchesDirect(t *testing.T) { ... }

// TestEQCurveSpriteRenderCount asserts steady-state frames do not re-render:
// drive 30 Update/Draw frames with no edits and assert the interceptor-
// counted drawRect calls inside the plot occur only on the first frame.
func TestEQCurveSpriteRenderCount(t *testing.T) { ... }
```

Implementation detail for both: swap the package `drawRect` var with a counting/recording wrapper (the `audio_panel_render_pixels_test.go` interceptor pattern — see `wasm_analyzer_bridge_stub.go` hooks), restore via `t.Cleanup`. For identity, compare `image.At` over the plot rect between (a) a frame drawn normally and (b) a frame drawn with `envRenderSafe=true` (both after identical state setup, fresh dst images cleared to the panel background first).

- [ ] **Step 5: Run.**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestEQCurveSprite' -count=1 ./internal/ui/`
Expected: PASS.

- [ ] **Step 6: Draw-call ratchet.** Add to the same file:

```go
// TestEQTabSteadyStateDrawCallBudget pins the win: a steady-state EQ-tab
// frame previously issued ~2 drawRect calls per plot column (plus stripes).
// With the sprite it must stay under 150 primitives per frame (chrome +
// handles + sliders; the cached content contributes ONE DrawImage).
func TestEQTabSteadyStateDrawCallBudget(t *testing.T) { ... } // count via drawRect interceptor over frames 5-10, assert max <= 150
```

Calibrate the 150 by running once and pinning measured+small headroom, citing the measured number in the comment (ratchet discipline).

- [ ] **Step 7: Regression gates.**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestEQ|TestAudioPanel|TestParity' -timeout 900s ./internal/ui/` and the pixel suite `-run 'TestAudioPanelRenderPixels'`.
Expected: PASS (RENDER_SAFE path untouched).

- [ ] **Step 8: gofmt; leave uncommitted.**

---

### Task 4: Spectrum adoption

**Files:**
- Modify: `src/go/internal/ui/eq_panel_zone.go:2267` (`drawSpectrumBars`), call site `:684`; also the Spectrum tab's hi-res curve underlay in `render_spectrum.go` (`drawAnalyzerSpectrumWithScale`/`drawSpectrumCurve`) — same treatment, one sprite per surface.
- Create: `src/go/internal/ui/spectrum_sprite_test.go`

**Interfaces:**
- Consumes: `TraceSprite`; analyzer snapshot identity.
- Produces: `EQPanelZone.spectrumSprite TraceSprite` (+ one for the Spectrum tab trace if it is a separate zone/method — implementer maps 1 sprite per draw surface).

- [ ] **Step 1:** Extract each surface's body into a `render*Content(img, r)` method exactly as Task 3 Step 1 (rect-relative; live cursor/chips stay out).
- [ ] **Step 2:** Key = `struct{ state *analyzer.State; slope int; pre bool; density Density; w, h int }` — the analyzer STATE POINTER is the freshness signal (the Chain tab precedent: a fresh `*State` arrives per analyzer tick; pointer comparison is the cheapest correct key). `minInterval = 0` (the tick is already ≤ ~30 Hz). Slope/Pre from the sticky-bar state (`SetSpectrumSlope`, Pre toggle) — fold both.
- [ ] **Step 3:** Identity + render-count tests mirroring Task 3 Step 4 (drive two analyzer ticks via `testAnalyzerStateOverride` — the hook `audio_panel_render_pixels_test.go` already uses — assert exactly 2 renders across N frames).
- [ ] **Step 4:** Run the spectrum + pixel + parity gates as in Task 3 Step 7. gofmt. Leave uncommitted.

---

### Task 5: Wave adoption

**Files:**
- Modify: `src/go/internal/ui/render_waveform.go:32` (`drawAnalyzerWaveform`) and its EQPanelZone call site (the Wave tab case in `(*EQPanelZone).Draw`).
- Create: `src/go/internal/ui/wave_sprite_test.go`

Same recipe as Task 4. `drawAnalyzerWaveform` is a free function taking `(dst, rect, ch, capture, beatGrid, gain, autoOn, tabFrozen)` — the sprite lives on the ZONE (owner), and the key folds: capture/state pointer identity, `gain`, `autoOn`, `tabFrozen`, beatGrid length + first/last values (cheap proxy; if the grid scrolls per frame, measure — if it genuinely changes every frame, key on it and accept per-tick renders, still bounded by the analyzer rate), density, w, h. The cursor crosshair (`drawWaveformCursor`) is live — stays at the call site, drawn after the sprite blit.

- [ ] Steps 1-4 as Task 4 (extract, key, tests, gates). Leave uncommitted.

---

### Task 6: Levels adoption

**Files:**
- Modify: `src/go/internal/ui/render_meters.go:447` (`drawLevelsMultiChannel`) + its call site.
- Create: `src/go/internal/ui/levels_sprite_test.go`

Levels data changes EVERY analyzer tick (meters move), so the sprite's win here is frames BETWEEN ticks (60 fps draw vs ~30 Hz data = ~2× fewer renders) plus call-count collapse. Key: `struct{ state *analyzer.State; latchGen uint64; k20 bool; density Density; w, h int }` — add a cheap generation counter to `MultiLevelsLatch` (increment on `Get` mutations/`Clear`) if none exists; if latch text mutates per tick anyway it is already covered by the state pointer. `minInterval = 0`.

- [ ] Steps as Task 4: extract to `renderLevelsContent`, key, identity + render-count tests (per-tick render, not per-frame), gates, gofmt. Leave uncommitted.
- [ ] **Alloc budget:** update `perTabAllocBudget[TabMeters]` in `eq_panel_draw_alloc_discipline_test.go` DOWNWARD with a one-line rationale citing the sprite (per the CLAUDE.md recipe).

---

### Task 7: Synth-tab right pane + SubImage handle cache (acceptance: byte budget green)

**Files:**
- Modify: `src/go/internal/ui/synth_panel_zone.go:1826` (`drawSynthTab`) region that renders the right preview pane + stage-card graphs (`synth_preview_layer.go` renderers); the SubImage call sites inside those renderers.
- Create: `src/go/internal/ui/synth_sprite_test.go`

**Interfaces:**
- Consumes: `TraceSprite`, `audio.GetInstrumentParams`/params-hash accessors already used by the mirror gate (`synthParamsHash`).
- Produces: sprites on `DrumView` (or the synth zone struct) for (a) the right preview pane, (b) each stage-card graph row if they render independently — implementer maps one sprite per contiguous cached surface; key `struct{ inst string; paramsHash uint64; section synthSectionID; density Density; lang string; w, h int }`, `minInterval = traceSpriteDragInterval` (knob drags).

- [ ] **Step 1:** Wrap the preview-pane + card-graph rendering in sprites (extract to `render*Content` as before). Live elements (knob positions themselves, drag highlights, chip strip "living" animations if frame-driven) stay out — anything animated per-frame must remain live or its animation freezes; identify by reading the renderers first and listing what depends on wall-clock vs params.
- [ ] **Step 2:** Inside the cached renderers, replace per-call `img.SubImage(r)` with a small memo on the owning struct: `subImages map[image.Rectangle]*ebiten.Image`, cleared in the sprite's render callback start (rects are stable within a layout). This kills the measured 92 MB/1 M-call churn during rebuilds too.
- [ ] **Step 3:** Tests: identity + render-count (as Task 3), PLUS the acceptance gate:

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestSynthTab_LiveKnobEditFrameByteBudget' -count=3 ./internal/ui/`
Expected: **PASS all 3 runs** (drag ≤ 224 KB/frame). This currently-failing test is the task's definition of done; do NOT raise its budget.

- [ ] **Step 4:** Full synth-tab test sweep: `-run 'TestSynthTab|TestSynthPanel|TestFocus' ...` — expect only the known `TestFocusGraph_RespondsToEveryKnob` flake unchanged. gofmt. Leave uncommitted.

---

### Task 8: Chain tab migrates onto TraceSprite

**Files:**
- Modify: `src/go/internal/ui/chain_panel_zone.go:140-160` (replace `traceCache *ebiten.Image` + `traceKey`/`traceKeyOK`/`traceState` bookkeeping with one `traceSprite TraceSprite`), and its render/blit site in `render_chain.go`.

- [ ] **Step 1:** Port: key = the existing `chainTraceKey` fields + the `scope.State` pointer (fold the pointer INTO the key struct — `struct{ k chainTraceKey; state *scope.State }`); `minInterval = 0`. Delete the bespoke fields.
- [ ] **Step 2:** Gate: `-run 'TestChain' ./internal/ui/` — `TestChainTraceCacheMatchesDirect` must pass UNMODIFIED (pixel-identity carries over). gofmt. Leave uncommitted.

---

### Task 9: Eviction at the tab-switch chokepoint

**Files:**
- Modify: `src/go/internal/ui/audio_panel_dispatcher.go:115` (`EnsureAnalyzersForTab`) — the single tab-change chokepoint (CLAUDE.md: "Don't add a fourth path").
- Create: `src/go/internal/ui/trace_sprite_eviction_test.go`

- [ ] **Step 1:** Give each owning zone a `releaseTraceSprites()` method (EQPanelZone: curve+spectrum+wave+levels sprites; synth pane; chain). In the dispatcher, on a tab CHANGE, call the deactivated tab's release via a new callbacks hook (mirror the existing `EQCallbacks` pattern — the dispatcher must not reach into zone privates).
- [ ] **Step 2:** Test: functional — open EQ tab, draw frames (sprite allocated), switch to Wave via `SetActiveTab`, assert the EQ zone's sprites report `img == nil` (add a tiny `func (s *TraceSprite) resident() bool { return s.img != nil }` for tests), switch back, assert re-render happens (render counter) and pixels return.
- [ ] **Step 3:** Gate: `-run 'TestZone|TestAudioPanelDispatcher|TestTraceSprite' ./internal/ui/`. gofmt. Leave uncommitted.

---

### Task 10: Suite-wide gates

- [ ] **Step 1:** Full fast suite: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -timeout 1500s ./...`
Expected: only `TestFocusGraph_RespondsToEveryKnob` (pre-existing flake) may fail; `TestSynthTab_LiveKnobEditFrameByteBudget` must be GREEN.
- [ ] **Step 2:** Builds: desktop CGO + `GOOS=js GOARCH=wasm` of `./cmd`.
- [ ] **Step 3:** Browser pixel harness: `GO=$(pwd)/.tools/go/bin/go node src/js/audio_panel_render.browser.test.js` (real WASM render content check) and `wasm_bridge_smoke.browser.test.js`.
Expected: PASS.

---

### Task 11: Measure the improvement (user-requested deliverable)

- [ ] **Step 1 (desktop):** `PPROF=1 PARITY_TIMING=log make bench BPM=120 SECS=50` + `curl -o after.pprof "http://localhost:6060/debug/pprof/profile?seconds=30"`; compare `Game.Draw`/`EQPanelZone.Draw`/`drawEQCurve` cum% against the baseline (28.4% / 3.26 s / 2.24 s from `scratchpad/draw_cpu.pprof`, recorded in the spec).
- [ ] **Step 2 (WASM):** `make wasm && GO=$(pwd)/.tools/go/bin/go node src/js/webaudio_perf_e2e.browser.test.js` — compare `drawDrumMS` (baseline 23.3 ms) and `drawCalls` (baseline 8,441). Machine must be otherwise idle (the baseline fps number was headless-noise; the per-frame stats are the comparables).
- [ ] **Step 3:** Record before/after in the completion notes + memory. If `drawDrumMS` did not land in low single digits, profile the residual before calling it done.

---

## Self-review notes

- Spec coverage: component+lifecycle (T2), RENDER_SAFE (T2), stub fidelity (T1), adopters EQ/Spectrum/Wave/Levels/Synth/Chain (T3-8), eviction chokepoint (T9), budgets+suites (T10), measurement (T11). Reusability doc = T2 type comment checklist.
- The one deliberate deviation from the skill template: no commit steps (user convention), and adopter tasks reference extraction of existing bodies by location instead of duplicating 200-line verbatim blocks — the extraction instruction + complete new orchestration code is the actionable content.
- Type consistency: `TraceSprite.Draw(dst, rect, key, render)` used identically in T3-T9; `traceSpriteDragInterval` defined T2, used T3/T7.
