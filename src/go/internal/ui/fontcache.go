//go:build !test

package ui

import (
	"image/color"
	"log"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text" //nolint:staticcheck // text.Draw has no drop-in v2 replacement yet
	"github.com/ingyamilmolinar/beatmo/internal/assets"
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
			log.Printf("fontcache: failed to parse Go Regular font: %v", err)
		}
		b, err := opentype.Parse(assets.GoBoldTTF)
		if err == nil {
			fontBold = b
		} else {
			log.Printf("fontcache: failed to parse Go Bold font: %v", err)
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

// measureTextWidth returns the pixel width of s rendered at the given size.
// Uses bold face to match the bold rendering in textSpriteRenderer.
func measureTextWidth(s string, size float64) int {
	face := fontFaceBold(size)
	if face == nil {
		return len([]rune(s)) * debugCharW
	}
	bounds := text.BoundString(face, s) //nolint:staticcheck // no drop-in replacement
	return bounds.Dx()
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

// renderTextSprite creates an image containing the text rendered with TrueType.
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
	bounds := text.BoundString(face, s) //nolint:staticcheck // no drop-in replacement
	iw := bounds.Dx() + 2
	ih := bounds.Dy() + 2
	if iw < 1 {
		iw = 1
	}
	if ih < 1 {
		ih = 1
	}
	img := ebiten.NewImage(iw, ih)
	// Draw at position that accounts for the bounds offset.
	text.Draw(img, s, face, -bounds.Min.X+1, -bounds.Min.Y+1, color.White)
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
}
