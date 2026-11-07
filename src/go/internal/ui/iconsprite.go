//go:build !test

package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

type iconSpriteKey struct {
	name string
	w, h int
}

var iconSpriteCache = map[iconSpriteKey]*ebiten.Image{}

// White-on-transparent, tinted via ColorScale. nil under go test so the icon var overrides keep working.
func iconSprite(name string, w, h int) *ebiten.Image {
	if runningUnderGoTest() || w <= 0 || h <= 0 {
		return nil
	}
	k := iconSpriteKey{name: name, w: w, h: h}
	if img, ok := iconSpriteCache[k]; ok {
		return img
	}
	img := ebiten.NewImage(w, h)
	if !drawIconByID(img, IconID(name), image.Rect(0, 0, w, h), color.White) {
		return nil
	}
	iconSpriteCache[k] = img
	return img
}
