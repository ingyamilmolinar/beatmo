//go:build !test

package ui

import (
	"image/color"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text" //nolint:staticcheck // text.Draw has no drop-in v2 replacement yet
	"github.com/ingyamilmolinar/beatmo/internal/assets"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
)

var (
	fontRegular  *opentype.Font
	fontBold     *opentype.Font
	fontInitOnce sync.Once

	// Face cache keyed by (bold, size*100) to avoid re-creating faces.
	faceCacheMu sync.Mutex
	faceCache   = map[uint64]font.Face{}
)

func initFontSources() {
	fontInitOnce.Do(func() {
		r, err := opentype.Parse(assets.GoRegularTTF)
		if err == nil {
			fontRegular = r
		} else {
			game_log.Errorf("[font] failed to parse Go Regular font: %v", err)
		}
		b, err := opentype.Parse(assets.GoBoldTTF)
		if err == nil {
			fontBold = b
		} else {
			game_log.Errorf("[font] failed to parse Go Bold font: %v", err)
		}
	})
}

func faceCacheKey(bold bool, size float64) uint64 {
	k := uint64(size * 100)
	if bold {
		k |= 1 << 63
	}
	return k
}

func fontFaceRegular(size float64) font.Face {
	initFontSources()
	if fontRegular == nil {
		return nil
	}
	key := faceCacheKey(false, size)
	faceCacheMu.Lock()
	if f, ok := faceCache[key]; ok {
		faceCacheMu.Unlock()
		return f
	}
	faceCacheMu.Unlock()
	face, err := opentype.NewFace(fontRegular, &opentype.FaceOptions{
		Size: size, DPI: 72, Hinting: font.HintingFull,
	})
	if err != nil {
		return nil
	}
	faceCacheMu.Lock()
	faceCache[key] = face
	faceCacheMu.Unlock()
	return face
}

func fontFaceBold(size float64) font.Face {
	initFontSources()
	if fontBold == nil {
		return nil
	}
	key := faceCacheKey(true, size)
	faceCacheMu.Lock()
	if f, ok := faceCache[key]; ok {
		faceCacheMu.Unlock()
		return f
	}
	faceCacheMu.Unlock()
	face, err := opentype.NewFace(fontBold, &opentype.FaceOptions{
		Size: size, DPI: 72, Hinting: font.HintingFull,
	})
	if err != nil {
		return nil
	}
	faceCacheMu.Lock()
	faceCache[key] = face
	faceCacheMu.Unlock()
	return face
}

// measureKey identifies one measurement: the string, the face (size + weight)
// and the font generation — keyed like the sprite caches, so a locale switch
// to another font never serves a stale width.
type measureKey struct {
	s    string
	size int // size*100
	bold bool
	gen  int64
}

// measureCacheMax bounds the width cache: the beat readout adds a string per
// second of playback; a full cache is simply dropped and refilled.
const measureCacheMax = 4096

// measuredWidths memoizes text advance widths. font.MeasureString walks every
// glyph's advance and kerning through the opentype face, and layout asks for
// the same strings every frame (tab pills, the beat readout, band labels) —
// on WASM that re-measuring was a third of the audio panel's per-frame layout.
var (
	measureMu      sync.RWMutex
	measuredWidths = map[measureKey]int{}
)

// textAdvance returns the advance width of s on the face for (size, bold).
func textAdvance(s string, size float64, bold bool) int {
	key := measureKey{s: s, size: int(size * 100), bold: bold, gen: i18n.FontGeneration()}
	measureMu.RLock()
	w, ok := measuredWidths[key]
	measureMu.RUnlock()
	if ok {
		return w
	}
	var face font.Face
	if bold {
		face = fontFaceBold(size)
	} else {
		face = fontFaceRegular(size)
	}
	if face == nil {
		return len([]rune(s)) * debugCharW
	}
	// Use the glyph ADVANCE, not text.BoundString's INK bounding box. A space has
	// no ink, so BoundString(" ").Dx()==0 and it drops leading/trailing-space
	// advances entirely — which made a leading space invisible: the caret (placed
	// at x+TextWidth(prefix)) never moved past it and the text never shifted, so
	// typing a space at the start looked like "nothing happened". MeasureString
	// sums per-glyph advances (the real layout width) and counts spaces.
	w = font.MeasureString(face, s).Ceil()
	measureMu.Lock()
	if len(measuredWidths) >= measureCacheMax {
		clear(measuredWidths)
	}
	measuredWidths[key] = w
	measureMu.Unlock()
	return w
}

// measureTextWidth returns the pixel width of s rendered at the given size.
// Uses bold face to match the bold rendering in textSpriteRenderer.
func measureTextWidth(s string, size float64) int {
	return textAdvance(s, size, true)
}

// measureTextHeight returns the line height for the given size.
// Uses bold face to match the bold rendering in textSpriteRenderer.
func measureTextHeight(size float64) int {
	face := fontFaceBold(size)
	if face == nil {
		return debugCharH
	}
	m := face.Metrics()
	return (m.Ascent + m.Descent).Ceil()
}

// measureStyled returns width+height for s at the given size and weight.
func measureStyled(s string, size float64, bold bool) (int, int) {
	var face font.Face
	if bold {
		face = fontFaceBold(size)
	} else {
		face = fontFaceRegular(size)
	}
	if face == nil {
		return len([]rune(s)) * debugCharW, styledLineHeightFallback(size)
	}
	m := face.Metrics()
	return textAdvance(s, size, bold), (m.Ascent + m.Descent).Ceil()
}

// textSpriteLayout computes the SHARED text-sprite geometry: pixel width/height,
// the horizontal pen origin, and the baseline row. This is the single source of
// truth every text input, label and button depends on for absolute positioning.
//
// Height and baseline come from the FIXED font metrics (ascent/descent), NOT the
// per-string ink bounds. That makes the text's vertical position ABSOLUTE and
// content-independent: a baseline-resting glyph sits at the same row whether or
// not the string also contains a taller uppercase/ascender glyph, so deleting a
// leading capital can no longer shift the whole string up or down.
//
// The pen origin is x=0, so the layout START coincides with the sprite's left
// edge — which is exactly the point DrawTextAt blits to screen (x,y). A caret
// drawn at x+TextWidth(prefix) therefore lands precisely on the glyph boundary,
// not 1px off. Width covers the layout advance (so a trailing space and the
// end-of-string caret have room), widened to the ink's right edge for the rare
// glyph that overhangs its advance.
func textSpriteLayout(face font.Face, s string) (w, h, penX, baseline int) {
	m := face.Metrics()
	baseline = m.Ascent.Ceil()
	h = (m.Ascent + m.Descent).Ceil() // == TextHeight(); the field-caret spans this
	w = font.MeasureString(face, s).Ceil()
	if b := text.BoundString(face, s); b.Max.X > w { //nolint:staticcheck // ink right-overhang only
		w = b.Max.X
	}
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	penX = 0
	return
}

// renderTextSprite creates an image containing the text rendered with TrueType,
// laid out via textSpriteLayout (fixed metric box + zero pen origin).
func renderTextSprite(s string, size float64, bold bool) *ebiten.Image {
	var face font.Face
	if bold {
		face = fontFaceBold(size)
	} else {
		face = fontFaceRegular(size)
	}
	if face == nil {
		return nil // fallback to debug font
	}
	w, h, penX, baseline := textSpriteLayout(face, s)
	img := ebiten.NewImage(w, h)
	text.Draw(img, s, face, penX, baseline, color.White)
	return img
}

func init() {
	// Replace the text sprite renderer with TrueType.
	textSpriteRenderer = func(s string) *ebiten.Image {
		img := renderTextSprite(s, FontSizeBody, true)
		if img != nil {
			return img
		}
		return nil // textcache.go handles nil by falling back to debug font
	}

	// Replace text measurement.
	textMeasureWidth = func(s string) int {
		return measureTextWidth(s, FontSizeBody)
	}
	textMeasureHeight = func() int {
		return measureTextHeight(FontSizeBody)
	}

	// Wire sized text-role renderer and measurement.
	styledTextSpriteRenderer = func(s string, size float64, bold bool) *ebiten.Image {
		return renderTextSprite(s, size, bold)
	}
	styledTextMeasure = func(s string, size float64, bold bool) (int, int) {
		return measureStyled(s, size, bold)
	}
}
