package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestIconSettingsDraws(t *testing.T) {
	dst := ebiten.NewImage(32, 32)
	if !DrawIcon(dst, IconSettings, image.Rect(0, 0, 32, 32), colTextPrimary) {
		t.Fatal("DrawIcon(IconSettings) returned false — icon not registered")
	}
}

func TestIconSettingsEmptyRectNoPanic(t *testing.T) {
	dst := ebiten.NewImage(8, 8)
	// Empty rect must not panic (every icon guards r.Empty()).
	_ = DrawIcon(dst, IconSettings, image.Rectangle{}, colTextPrimary)
}
