//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// The Spacious (mobile) icon-readout column must be narrow enough to leave the
// bars room. Pin the reduced token value.
func TestSpaciousLevelsReadoutWIconsReduced(t *testing.T) {
	restore := SetDensityForTest(DensitySpacious)
	defer restore()
	got := Profile().DensityValues().LevelsReadoutWIcons
	if got > 34 {
		t.Fatalf("spacious LevelsReadoutWIcons should be <= 34 for mobile bar room; got %d", got)
	}
}

// At a typical mobile panel width the Levels renderer must still draw (icon
// mode), leaving the bars the rest of the width. Smoke check that the narrowed
// column does not break rendering.
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
