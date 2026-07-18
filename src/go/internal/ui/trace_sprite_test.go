//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

// Stub-backend only (mirrors audio_panel_render_pixels_test.go's gate), for
// two independent reasons — both of which make these assertions meaningless,
// not merely unavailable, under the real Ebiten backend `make test-real`
// uses:
//
//  1. Pixel reads. The blit assertions call dst.At(), and real Ebiten's
//     ReadPixels panics outright ("cannot be called before the game starts")
//     because there is no started game loop / graphics driver to read back
//     from. Verifying pixels via At() on an offscreen image is a capability
//     the ebitestub backend deliberately provides (see the SubImage note in
//     drumview_tree.go's Draw doc) and real Ebiten deliberately does not.
//     Being a panic rather than a failure, it also aborted the whole
//     internal/ui binary, masking every test ordered after it.
//
//  2. The alloc budget. TestTraceSpriteSteadyStateDoesNotAllocate's "<=1
//     alloc/op" ceiling is calibrated against the stub's DrawImage; real
//     Ebiten's allocates 3/op, so the number only means anything here.
//
// The cache's logic (render-once-per-key, throttle, invalidate/release,
// RENDER_SAFE bypass) is backend-independent pure Go, so it loses no real
// coverage by running against the stub alone.

// countingRender returns a render func that fills its rect with a solid
// color and counts invocations.
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

	// Churn: a new key every simulated 16ms frame, 6 frames (t=0..80ms).
	for i := 0; i < 6; i++ {
		s.Draw(dst, rect, i, render)
		now = now.Add(16 * time.Millisecond)
	}
	// At a 33ms cap: renders at t=0 (cold), t=48 (first frame ≥33 after),
	// then t=96 would be next — expect exactly 2 within the churn window.
	if calls != 2 {
		t.Fatalf("throttled churn: want 2 renders, got %d", calls)
	}
	// Trailing edge: the final key (5) was never rendered during churn; the
	// first Draw after the interval elapses must render it exactly once.
	now = now.Add(40 * time.Millisecond)
	s.Draw(dst, rect, 5, render)
	if calls != 3 {
		t.Fatalf("trailing edge: want final-state render, got %d calls", calls)
	}
	s.Draw(dst, rect, 5, render)
	if calls != 3 {
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
	if s.resident() {
		t.Fatal("Release must drop the texture handle")
	}
	s.Draw(dst, rect, "k", render) // must re-allocate and render, not panic
	if calls != 3 {
		t.Fatalf("post-Release Draw: want re-render, got %d", calls)
	}
	if !s.resident() {
		t.Fatal("post-Release Draw must re-allocate the texture")
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
	if s.resident() {
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
	// The key literal is a constant string (no boxing allocation); the blit
	// itself must be alloc-free.
	if avg > 1 {
		t.Fatalf("steady-state Draw allocates %.1f allocs/op (want <=1)", avg)
	}
}
