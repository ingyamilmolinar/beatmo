package ui

import (
	"image/color"
	"sync"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
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

// Simple text sprite cache. Keyed by exact string + font generation so a
// locale switch to a different font face (which bumps the generation) never
// serves a stale sprite.
type textKey struct {
	s   string
	gen int64
}

var (
	textCacheMu sync.RWMutex
	textSprites = map[textKey]*ebiten.Image{}
)

// TextSprite returns a cached image containing the provided text rendered
// using TrueType fonts (when available) or Ebiten's debug font as fallback.
func TextSprite(s string) *ebiten.Image {
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
		// The TrueType renderer is unavailable, which only happens if the
		// embedded font failed to initialise -- a state in which no glyph
		// source exists to draw from. Emit a correctly-sized blank sprite
		// rather than importing ebitenutil, whose sibling NewImageFromURL
		// drags net/http + crypto/tls (~4 MiB) into the browser build. Under
		// -tags test this path was already a no-op stub (ebitestub's
		// DebugPrintAt), so behaviour there is unchanged.
		img = ebiten.NewImage(w, h)
	}

	textCacheMu.Lock()
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

// TextRole is a semantic typography role. Each role maps to a true pixel size
// and weight; StyledText* renders at that size directly (no GeoM upscale), so
// glyphs stay crisp and the panel-title/section-header/body/caption hierarchy
// is real. Phase 12 aligns DESIGN.md's typography block (font family + px sizes) to these roles.
type TextRole int

const (
	RoleBody          TextRole = iota // 14px Inter SemiBold — menu item labels, values
	RoleCaption                       // 12px Inter Regular  — sub-labels ("Vol", "Pct")
	RoleSectionHeader                 // 16px Inter SemiBold — section labels
	RolePanelTitle                    // 21px Inter SemiBold — panel/menu titles
)

func (r TextRole) size() float64 {
	switch r {
	case RolePanelTitle:
		return 21
	case RoleSectionHeader:
		return 16
	case RoleCaption:
		return 12
	default:
		return 14
	}
}

func (r TextRole) bold() bool { return r != RoleCaption }

// styledLineHeightFallback is the build-agnostic line-height estimate used
// under -tags test (no TTF available). Monotonic in size so the role
// hierarchy holds without a font backend.
func styledLineHeightFallback(size float64) int { return int(size*1.25) + 1 }

// styledTextSpriteRenderer / styledTextMeasure are wired by fontcache.go in
// non-test builds. nil under -tags test → debug-font / arithmetic fallback.
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
	styledSprites[key] = img
	styledCacheMu.Unlock()
	return img
}

// DrawTextStyled draws s at (x,y) in the given role, tinted with col. The
// sprite is rendered at true px size — blit 1:1, no GeoM scaling.
func DrawTextStyled(dst *ebiten.Image, s string, x, y int, role TextRole, col color.Color) {
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

// drawTextColorAtScaleHook, when non-nil, is invoked with each string passed
// to DrawTextColorAtScale. Test-only observation seam (zero cost when nil).
var drawTextColorAtScaleHook func(string)

// DrawTextColorAtScale draws cached text at (x,y) scaled and tinted with col.
// Combines GeoM.Scale for size and ColorScale for tint in a single blit.
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
