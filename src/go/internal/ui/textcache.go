package ui

import (
	"image/color"
	"sync"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// Here, not fontcache.go, so test builds see them.
const (
	FontSizeCaption float64 = 10
	FontSizeSmall   float64 = 12
	FontSizeBody    float64 = 14
	FontSizeLabel   float64 = 16
	FontSizeTitle   float64 = 18
	FontSizeHeading float64 = 20
)

// fontcache.go installs the TrueType renderer; test builds use the debug font.
var textSpriteRenderer func(s string) *ebiten.Image

var textMeasureWidth func(s string) int

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

// Keyed by font generation too, so a locale switch to another face never serves a stale sprite.
type textKey struct {
	s   string
	gen int64
}

// A text input caches a sprite per DISTINCT string, so typing would grow the caches without bound; a full clear on
// overflow is cheap and rare.
const textCacheMaxEntries = 4096

var (
	textCacheMu sync.RWMutex
	textSprites = map[textKey]*ebiten.Image{}
)

// textCacheLen / styledCacheLen report the current cache sizes (test seam).
func textCacheLen() int {
	textCacheMu.RLock()
	defer textCacheMu.RUnlock()
	return len(textSprites)
}

func styledCacheLen() int {
	styledCacheMu.RLock()
	defer styledCacheMu.RUnlock()
	return len(styledSprites)
}

// Test seam (zero cost when nil): every drawn string passes through it; the tour's no-"tetris" guard reads it.
var textSpriteHook func(string)

func TextSprite(s string) *ebiten.Image {
	if textSpriteHook != nil {
		textSpriteHook(s)
	}
	k := textKey{s: s, gen: i18n.FontGeneration()}
	textCacheMu.RLock()
	if spr := textSprites[k]; spr != nil {
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
		// Only when the embedded font failed. A blank sprite instead of ebitenutil, whose NewImageFromURL drags net/http +
		// crypto/tls (~4 MiB) into the browser build.
		img = ebiten.NewImage(w, h)
	}

	textCacheMu.Lock()
	if len(textSprites) >= textCacheMaxEntries {
		textSprites = map[textKey]*ebiten.Image{}
	}
	textSprites[k] = img
	textCacheMu.Unlock()
	return img
}

// ClearTextCacheForTest clears the cache to provide a clean slate in tests.
func ClearTextCacheForTest() {
	textCacheMu.Lock()
	textSprites = map[textKey]*ebiten.Image{}
	textCacheMu.Unlock()
	styledCacheMu.Lock()
	styledSprites = map[styledKey]*ebiten.Image{}
	styledCacheMu.Unlock()
}

// Each role is a true pixel size, rendered without GeoM upscaling, so glyphs stay crisp.
type TextRole int

const (
	RoleBody          TextRole = iota // 14px Inter SemiBold — menu item labels, values
	RoleCaption                       // 12px Inter Regular  — sub-labels ("Vol", "Pct")
	RoleSectionHeader                 // 16px Inter SemiBold — section labels
	RolePanelTitle                    // 21px Inter SemiBold — panel/menu titles
	RoleMicro                         // 10px Inter Regular  — dense data-canvas labels (axis ticks, meter scales)
)

func (r TextRole) size() float64 {
	switch r {
	case RolePanelTitle:
		return 21
	case RoleSectionHeader:
		return 16
	case RoleCaption:
		return 12
	case RoleMicro:
		return 10
	default:
		return 14
	}
}

func (r TextRole) bold() bool { return r != RoleCaption && r != RoleMicro }

// Monotonic in size so the role hierarchy holds without a font backend.
func styledLineHeightFallback(size float64) int { return int(size*1.25) + 1 }

// nil under -tags test (debug-font / arithmetic fallback).
var (
	styledTextSpriteRenderer func(s string, size float64, bold bool) *ebiten.Image
	styledTextMeasure        func(s string, size float64, bold bool) (int, int)
)

type styledKey struct {
	s    string
	size int // size*100, integer key
	bold bool
	gen  int64
}

var (
	styledCacheMu sync.RWMutex
	styledSprites = map[styledKey]*ebiten.Image{}
)

// StyledTextWidth returns the rendered pixel width of s at the given role.
func StyledTextWidth(s string, role TextRole) int {
	if styledTextMeasure != nil {
		w, _ := styledTextMeasure(s, role.size(), role.bold())
		return w
	}
	return debugCharW * utf8.RuneCountInString(s)
}

// StyledTextHeight returns the line height for the given role.
func StyledTextHeight(role TextRole) int {
	if styledTextMeasure != nil {
		_, h := styledTextMeasure("Ag", role.size(), role.bold())
		if h > 0 {
			return h
		}
	}
	return styledLineHeightFallback(role.size())
}

// StyledTextSprite returns a cached sprite for s at role's size+weight.
func StyledTextSprite(s string, role TextRole) *ebiten.Image {
	if textSpriteHook != nil {
		textSpriteHook(s)
	}
	key := styledKey{s: s, size: int(role.size() * 100), bold: role.bold(), gen: i18n.FontGeneration()}
	styledCacheMu.RLock()
	if spr := styledSprites[key]; spr != nil {
		styledCacheMu.RUnlock()
		return spr
	}
	styledCacheMu.RUnlock()

	var img *ebiten.Image
	if styledTextSpriteRenderer != nil {
		img = styledTextSpriteRenderer(s, role.size(), role.bold())
	}
	if img == nil {
		img = TextSprite(s)
	}
	styledCacheMu.Lock()
	if len(styledSprites) >= textCacheMaxEntries {
		styledSprites = map[styledKey]*ebiten.Image{}
	}
	styledSprites[key] = img
	styledCacheMu.Unlock()
	return img
}

// Blitted 1:1, no scaling.
func DrawTextStyled(dst *ebiten.Image, s string, x, y int, role TextRole, col color.Color) {
	if drawTextStyledHook != nil {
		drawTextStyledHook(s)
	}
	if drawTextStyledPosHook != nil {
		drawTextStyledPosHook(s, x, y)
	}
	spr := StyledTextSprite(s, role)
	var op ebiten.DrawImageOptions
	op.GeoM.Translate(float64(x), float64(y))
	r, g, b, a := col.RGBA()
	if a > 0 {
		fa := float64(a) / 0xffff
		op.ColorScale.Scale(float32(float64(r)/0xffff/fa), float32(float64(g)/0xffff/fa), float32(float64(b)/0xffff/fa), float32(fa))
	}
	dst.DrawImage(spr, &op)
}

// DrawTextAt draws cached text at screen position (x,y) using a cached sprite.
func DrawTextAt(dst *ebiten.Image, s string, x, y int) {
	spr := TextSprite(s)
	var op ebiten.DrawImageOptions
	op.GeoM.Translate(float64(x), float64(y))
	dst.DrawImage(spr, &op)
}

// The sprites are white-on-transparent, so ColorScale tints them.
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

// Scaling at blit time, so the existing sprite is reused.
func DrawTextAtScale(dst *ebiten.Image, s string, x, y int, scale float64) {
	spr := TextSprite(s)
	var op ebiten.DrawImageOptions
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(float64(x), float64(y))
	dst.DrawImage(spr, &op)
}

// Test seam, zero cost when nil.
var drawTextColorAtScaleHook func(string)

// Test seam (zero cost when nil), the styled-path sibling of drawTextColorAtScaleHook.
var drawTextStyledHook func(string)

// Test seam (zero cost when nil) with the draw origin, for the barrel-lane ordering guards.
var drawTextStyledPosHook func(s string, x, y int)

// Scale and tint in one blit.
func DrawTextColorAtScale(dst *ebiten.Image, s string, x, y int, col color.Color, scale float64) {
	if drawTextColorAtScaleHook != nil {
		drawTextColorAtScaleHook(s)
	}
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
