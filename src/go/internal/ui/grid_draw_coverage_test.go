//go:build test

package ui

import (
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestGridCachePadAreaCoveredAtMinZoom verifies that after drawGridPane at
// minimum camera scale the grid cache pixel at (0,0) is non-zero.
//
// At cam.Scale=0.1 the tile width is 6px.  The old formula
//
//	startX = gridCachePad + phaseX - tileW  (= 64+0-6 = 58)
//
// leaves cache pixels x=0..57 untiled (blank), so At(0,0) == color.RGBA{}.
// The fix ensures startX <= -1 via the modulo formula.
func TestGridCachePadAreaCoveredAtMinZoom(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 360)

	// Minimum camera scale → tileW = round(0.1 * 60) = 6
	g.cam.Scale = 0.1
	g.cam.OffsetX = 0
	g.cam.OffsetY = 0
	g.cam.Snap()

	dst := ebiten.NewImage(640, g.split.Y)
	g.drawGridPane(dst)

	if g.gridCache == nil {
		t.Fatal("gridCache was not built")
	}

	got := color.RGBAModel.Convert(g.gridCache.At(0, 0)).(color.RGBA)
	if got == (color.RGBA{}) {
		t.Fatalf("gridCache.At(0,0) is zero (untiled pad gap); expected non-zero grid-line pixel. "+
			"tileW=6, old startX=58 left x=0..57 blank. gridCachePad=%d", g.gridCachePad)
	}
}

// TestGridCacheGapZoneAbsentAtMinZoom verifies that the entire old gap zone
// (x=0 to gapEnd, y=0) in the grid cache is covered with non-zero pixels
// after drawGridPane at minimum scale.
//
// With the buggy formula, gapEnd = gridCachePad + phaseX - tileW - 1 = 57.
// Pixels x=0..57 at y=0 are blank.  After the fix all are tiled.
func TestGridCacheGapZoneAbsentAtMinZoom(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 360)

	g.cam.Scale = 0.1
	g.cam.OffsetX = 0
	g.cam.OffsetY = 0
	g.cam.Snap()

	dst := ebiten.NewImage(640, g.split.Y)
	g.drawGridPane(dst)

	if g.gridCache == nil {
		t.Fatal("gridCache was not built")
	}

	// tileW=6, gridCachePad=64, phaseX=0 → old gap zone x=0..57
	tileW := g.gridTile.Bounds().Dx()
	gapEnd := g.gridCachePad - tileW - 1 // = 64 - 6 - 1 = 57

	for x := 0; x <= gapEnd; x++ {
		got := color.RGBAModel.Convert(g.gridCache.At(x, 0)).(color.RGBA)
		if got == (color.RGBA{}) {
			t.Fatalf("gridCache.At(%d,0) is zero (untiled gap); expected non-zero. "+
				"tileW=%d gridCachePad=%d gapEnd=%d", x, tileW, g.gridCachePad, gapEnd)
		}
	}
}

// TestGridCacheGapZoneAbsentAtDefaultZoom verifies the gap at cam.Scale=1.0
// with a camera offset that maximises the buggy gap.
//
// With cam.OffsetX=50: phaseX=50, old startX = 64+50-60 = 54.
// Cache pixels x=0..53 are blank with the old formula; x=53 should be
// non-zero after the fix.
func TestGridCacheGapZoneAbsentAtDefaultZoom(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 360)

	g.cam.Scale = 1.0
	g.cam.OffsetX = 50
	g.cam.OffsetY = 0
	g.cam.Snap()

	dst := ebiten.NewImage(640, g.split.Y)
	g.drawGridPane(dst)

	if g.gridCache == nil {
		t.Fatal("gridCache was not built")
	}

	// tileW=60, gridCachePad=64, phaseX=50 → old startX=54 → gap x=0..53
	checkX := 53 // last pixel in old gap zone
	got := color.RGBAModel.Convert(g.gridCache.At(checkX, 0)).(color.RGBA)
	if got == (color.RGBA{}) {
		t.Fatalf("gridCache.At(%d,0) is zero (untiled pad gap); expected non-zero. "+
			"tileW=60 phaseX=50 old startX=54 left x=0..53 blank. gridCachePad=%d",
			checkX, g.gridCachePad)
	}
}
