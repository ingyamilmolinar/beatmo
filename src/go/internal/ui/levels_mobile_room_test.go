//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Narrow enough that the bars keep room.
func TestSpaciousLevelsReadoutWIconsReduced(t *testing.T) {
	restore := SetDensityForTest(DensitySpacious)
	defer restore()
	got := Profile().DensityValues().LevelsReadoutWIcons
	if got > 34 {
		t.Fatalf("spacious LevelsReadoutWIcons should be <= 34 for mobile bar room; got %d", got)
	}
}

// Still draws in icon mode at a typical mobile width.
func TestLevelsMobileReadoutModeLeavesBarRoom(t *testing.T) {
	restore := SetDensityForTest(DensitySpacious)
	defer restore()

	rect := image.Rect(0, 0, 390, 160) // typical mobile panel width
	dst := ebiten.NewImage(rect.Dx(), rect.Dy())

	labels := collectDrawnTexts(t, func() {
		drawLevelsMultiChannel(dst, rect, filterState(), NewMultiLevelsLatch(), nil)
	})
	if len(labels) == 0 {
		t.Fatalf("expected some labels drawn at mobile width")
	}
}
