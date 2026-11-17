package ui

import (
	"image"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

// traceSpriteDragInterval is the re-render cap for surfaces whose key churns
// continuously during a drag (EQ handle, synth knob): ~30 Hz, user-approved
// staleness (2026-07-13 trace-sprite design). Event-driven surfaces use 0
// (re-render on every change).
const traceSpriteDragInterval = 33 * time.Millisecond

// TraceSprite memoizes a rect-sized rendering into an offscreen GPU texture,
// re-rendering only when the caller's version key changes (optionally rate-
// capped) and blitting ONE image per frame otherwise.
//
// Why: the audio panel's data surfaces rendered as thousands of one-pixel
// drawRect->DrawImage calls per frame (drawEQCurve alone measured 13.8% of
// process CPU; 8,441 draw calls/frame on WASM). Ebiten merges consecutive
// same-source draws into one GPU command, so the cost is per-call Go-side
// overhead — and on WASM it runs on the single thread the audio scheduler
// shares. A cached blit hands that thread back. See
// docs/superpowers/specs/2026-07-13-trace-sprite-cache-design.md.
//
// Adoption checklist (any widget whose pixels are a pure function of
// identifiable inputs can adopt this):
//  1. Fold EVERY render input into `key` — data version/hash, density,
//     language, selection state... anything the pixels depend on. The key
//     must be a comparable value (a small struct of comparables). Rect SIZE
//     is handled internally; rect position is not part of the content.
//  2. Render in rect-relative coordinates: the callback receives the target
//     image and the rect to draw within (at origin when cached; the original
//     rect under the RENDER_SAFE bypass). Existing rect-parameter helpers
//     work unchanged.
//  3. Keep live per-frame elements (cursors, drag handles, wall-clock
//     animations) OUT of the callback — draw them on top after Draw returns.
//  4. Release() when the owning surface hides (tab switch): textures are
//     reclaimed via ebiten.Image.Deallocate immediately instead of waiting
//     for a Go GC finalizer cycle (the game can run 20+ s between GCs).
//
// Not goroutine-safe: Draw/Invalidate/Release run on the UI goroutine only,
// like every other draw helper in this package.
type TraceSprite struct {
	img   *ebiten.Image
	key   any
	keyOK bool
	size  image.Point
	// tag names this sprite in the imagesByTag allocation metrics
	// (image_metrics.go). Empty = "traceSprite".
	tag string

	lastRender time.Time
	// minInterval rate-caps re-renders while the key churns (0 = re-render
	// on every change). The trailing edge is guaranteed without timers:
	// Draw runs every frame, so the first frame after the interval elapses
	// re-renders the final key.
	minInterval time.Duration
	// now is injectable for deterministic throttle tests; nil = time.Now.
	now func() time.Time
	// themeGen is the ThemeGeneration the texture was rendered under. Every
	// TraceSprite bakes token colors, so the theme is an implicit part of
	// EVERY caller's key — checked here once instead of trusting each
	// adopter to fold it in (the EQ bar backdrop kept the previous theme's
	// colors after a Theme-editor pick).
	themeGen int
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
		releaseImage(s.img)
		tag := s.tag
		if tag == "" {
			tag = "traceSprite"
		}
		s.img = newTrackedImage(tag, rect.Dx(), rect.Dy())
		s.size = rect.Size()
		// A stale-size blit would smear: force the render below regardless
		// of the throttle by dropping key validity.
		s.keyOK = false
	}
	if tg := ThemeGeneration(); s.themeGen != tg {
		s.themeGen = tg
		s.keyOK = false // immediate: an invalid key bypasses the throttle
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
	bumpDrawCall()
	drawRectOp.GeoM.Reset()
	drawRectOp.GeoM.Translate(float64(rect.Min.X), float64(rect.Min.Y))
	dst.DrawImage(s.img, &drawRectOp)
}

// Invalidate forces the next Draw to re-render. It bypasses key equality AND
// the churn throttle (the throttle only applies while the cached key is
// valid), so a forced render is immediate.
func (s *TraceSprite) Invalidate() { s.keyOK = false }

// Release deallocates the texture immediately and clears the cache state.
// The sprite stays usable: the next Draw re-allocates. Call on tab hide.
func (s *TraceSprite) Release() {
	releaseImage(s.img)
	s.img = nil
	s.size = image.Point{}
	s.keyOK = false
}

// resident reports whether a texture is currently allocated. Test hook for
// eviction assertions.
func (s *TraceSprite) resident() bool { return s.img != nil }
