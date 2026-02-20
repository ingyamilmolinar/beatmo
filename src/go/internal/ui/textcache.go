package ui

import (
	"image/color"
	"sync"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

// Font size constants for the type scale. Defined here (not in fontcache.go)
// so they are accessible in both production and test builds.
const (
	FontSizeCaption float64 = 10
	FontSizeSmall   float64 = 12
	FontSizeBody    float64 = 14
	FontSizeLabel   float64 = 16
	FontSizeTitle   float64 = 18
	FontSizeHeading float64 = 20
)

// textSpriteRenderer is the pluggable function that creates text sprite images.
// When a TrueType font is available (non-test builds), fontcache.go replaces
// this with a proper font renderer. Test builds use the debug font fallback.
var textSpriteRenderer func(s string) *ebiten.Image

// textMeasureWidth measures the pixel width of text. Replaced by fontcache.go
// in non-test builds for proper font metrics.
var textMeasureWidth func(s string) int

// textMeasureHeight returns the line height of body text. Replaced by
// fontcache.go in non-test builds for proper font metrics.
var textMeasureHeight func() int

// TextWidth returns the rendered width of s in pixels at body text size.
func TextWidth(s string) int {
	if textMeasureWidth != nil {
		return textMeasureWidth(s)
	}
	return debugCharW * utf8.RuneCountInString(s)
}

// TextHeight returns the body text line height in pixels.
func TextHeight() int {
	if textMeasureHeight != nil {
		return textMeasureHeight()
	}
	return debugCharH
}

// Simple text sprite cache. Keys are exact strings.
var (
	textCacheMu sync.RWMutex
	textSprites = map[string]*ebiten.Image{}
)

// TextSprite returns a cached image containing the provided text rendered
// using TrueType fonts (when available) or Ebiten's debug font as fallback.
func TextSprite(s string) *ebiten.Image {
	textCacheMu.RLock()
	if spr := textSprites[s]; spr != nil {
		textCacheMu.RUnlock()
		return spr
	}
	textCacheMu.RUnlock()

	var img *ebiten.Image

	// Try TrueType renderer first.
	if textSpriteRenderer != nil {
		img = textSpriteRenderer(s)
	}

	// Fallback to debug font.
	if img == nil {
		w := debugCharW * utf8.RuneCountInString(s)
		h := debugCharH
		if w < 1 {
			w = 1
		}
		if h < 1 {
			h = 1
		}
		img = ebiten.NewImage(w, h)
		ebitenutil.DebugPrintAt(img, s, 0, 0)
	}

	textCacheMu.Lock()
	textSprites[s] = img
	textCacheMu.Unlock()
	return img
}

// ClearTextCacheForTest clears the cache to provide a clean slate in tests.
func ClearTextCacheForTest() {
	textCacheMu.Lock()
	textSprites = map[string]*ebiten.Image{}
	textCacheMu.Unlock()
}

// DrawTextAt draws cached text at screen position (x,y) using a cached sprite.
func DrawTextAt(dst *ebiten.Image, s string, x, y int) {
	spr := TextSprite(s)
	var op ebiten.DrawImageOptions
	op.GeoM.Translate(float64(x), float64(y))
	dst.DrawImage(spr, &op)
}

// DrawTextColorAt draws cached text at screen position (x,y) tinted with col.
// Text sprites are white-on-transparent, so color scaling tints them.
func DrawTextColorAt(dst *ebiten.Image, s string, x, y int, col color.Color) {
	spr := TextSprite(s)
	var op ebiten.DrawImageOptions
	op.GeoM.Translate(float64(x), float64(y))
	r, g, b, a := col.RGBA()
	if a > 0 {
		fa := float64(a) / 0xffff
		op.ColorScale.Scale(float32(float64(r)/0xffff/fa), float32(float64(g)/0xffff/fa), float32(float64(b)/0xffff/fa), float32(fa))
	}
	dst.DrawImage(spr, &op)
}

// DrawTextAtScale draws cached text at screen position (x,y) scaled by the
// given factor. Reuses the existing sprite cache — scaling is applied at blit
// time via GeoM.Scale so no extra allocations occur.
func DrawTextAtScale(dst *ebiten.Image, s string, x, y int, scale float64) {
	spr := TextSprite(s)
	var op ebiten.DrawImageOptions
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(float64(x), float64(y))
	dst.DrawImage(spr, &op)
}

// DrawTextColorAtScale draws cached text at (x,y) scaled and tinted with col.
// Combines GeoM.Scale for size and ColorScale for tint in a single blit.
func DrawTextColorAtScale(dst *ebiten.Image, s string, x, y int, col color.Color, scale float64) {
	spr := TextSprite(s)
	var op ebiten.DrawImageOptions
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(float64(x), float64(y))
	r, g, b, a := col.RGBA()
	if a > 0 {
		fa := float64(a) / 0xffff
		op.ColorScale.Scale(float32(float64(r)/0xffff/fa), float32(float64(g)/0xffff/fa), float32(float64(b)/0xffff/fa), float32(fa))
	}
	dst.DrawImage(spr, &op)
}
