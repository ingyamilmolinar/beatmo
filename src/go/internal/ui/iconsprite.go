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

// iconSprite returns a cached white-on-transparent sprite for the named icon
// at the given dimensions. Returns nil for unknown icon names or when running
// in a go test binary (to allow icon draw function var overrides). The sprite
// is rendered once and reused; callers tint it to the target color via
// ColorScale.
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
