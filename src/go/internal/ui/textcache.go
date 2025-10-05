package ui

import (
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

// Simple text sprite cache for ebitenutil.DebugPrintAt text. Keys are exact strings.
var (
	textCacheMu sync.RWMutex
	textSprites = map[string]*ebiten.Image{}
)

// TextSprite returns a cached image containing the provided text rendered using
// Ebiten's default debug font. The sprite is drawn at (0,0) and sized to fit
// the text tightly based on the known glyph size.
func TextSprite(s string) *ebiten.Image {
	textCacheMu.RLock()
	if spr := textSprites[s]; spr != nil {
		textCacheMu.RUnlock()
		return spr
	}
	textCacheMu.RUnlock()
	// Create sprite sized to the debug font metrics.
	w := debugCharW * len([]rune(s))
	h := debugCharH
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	img := ebiten.NewImage(w, h)
	// Render once into the sprite.
	ebitenutil.DebugPrintAt(img, s, 0, 0)
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
// This avoids re-rendering glyphs every frame via ebitenutil.DebugPrintAt.
func DrawTextAt(dst *ebiten.Image, s string, x, y int) {
	spr := TextSprite(s)
	var op ebiten.DrawImageOptions
	op.GeoM.Translate(float64(x), float64(y))
	dst.DrawImage(spr, &op)
}
