package ui

import (
	"image"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

// ~30 Hz, an approved staleness for surfaces whose key churns during a drag; event-driven surfaces use 0.
const traceSpriteDragInterval = 33 * time.Millisecond

// Thousands of 1 px draw calls per frame cost Go-side overhead on WASM's single thread, which the audio scheduler
// shares. Fold EVERY render input into key, draw rect-relative, keep live elements out of render, Release on hide.
type TraceSprite struct {
	img   *ebiten.Image
	key   any
	keyOK bool
	size  image.Point
	// Empty means "traceSprite".
	tag string

	lastRender time.Time
	// 0 re-renders on every change; the trailing edge needs no timer, since Draw runs every frame.
	minInterval time.Duration
	// now is injectable for deterministic throttle tests; nil = time.Now.
	now func() time.Time
	// Every sprite bakes token colours, so the theme is an implicit key of every caller, checked here once instead of
	// trusting each adopter.
	themeGen int
}

func (s *TraceSprite) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func (s *TraceSprite) Draw(dst *ebiten.Image, rect image.Rectangle, key any, render func(img *ebiten.Image, r image.Rectangle)) {
	if dst == nil || render == nil || rect.Dx() <= 0 || rect.Dy() <= 0 {
		return
	}
	if envRenderSafe {
		// Every primitive must reach the instrumented drawRect on the real destination.
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
		// A stale-size blit would smear, so the render is forced past the throttle.
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
			// A panicking render re-renders next frame instead of blitting a half-drawn cache.
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

// Bypasses key equality AND the throttle, so the render is immediate.
func (s *TraceSprite) Invalidate() { s.keyOK = false }

// Frees the texture at once instead of at the next GC (which can be 20+ s away); the next Draw re-allocates.
func (s *TraceSprite) Release() {
	releaseImage(s.img)
	s.img = nil
	s.size = image.Point{}
	s.keyOK = false
}

// Test hook.
func (s *TraceSprite) resident() bool { return s.img != nil }
