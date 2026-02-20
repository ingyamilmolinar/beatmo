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
	r := image.Rect(0, 0, w, h)
	switch name {
	case "play":
		drawPlayIcon(img, r, color.White)
	case "pause":
		drawPauseIcon(img, r, color.White)
	case "stop":
		drawStopIcon(img, r, color.White)
	case "record":
		drawRecordIcon(img, r, color.White)
	case "pencil":
		drawPencilIcon(img, r, color.White)
	case "save":
		drawSaveIcon(img, r, color.White)
	case "close":
		drawCloseIcon(img, r, color.White)
	case "overflow":
		drawOverflowIcon(img, r, color.White)
	case "plus":
		drawPlusIcon(img, r, color.White)
	case "minus":
		drawMinusIcon(img, r, color.White)
	case "rows":
		drawRowsIcon(img, r, color.White)
	case "audio":
		drawAudioIcon(img, r, color.White)
	case "chevron-up":
		drawChevronUpIcon(img, r, color.White)
	case "chevron-down":
		drawChevronDownIcon(img, r, color.White)
	case "track":
		drawTrackIcon(img, r, color.White)
	case "track-off":
		drawTrackOffIcon(img, r, color.White)
	case "upload":
		drawUploadIcon(img, r, color.White)
	case "import":
		drawImportIcon(img, r, color.White)
	case "export":
		drawExportIcon(img, r, color.White)
	default:
		return nil
	}
	iconSpriteCache[k] = img
	return img
}
