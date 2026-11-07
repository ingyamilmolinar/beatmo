//go:build test

package ui

import (
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// At cam.Scale=0.1 the tile is 6 px; startX must be ≤ -1, or cache columns 0..57 stay untiled.
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

	if !gridColumnCovered(g.gridCache, 0, g.gridTile.Bounds().Dx()) {
		t.Fatalf("gridCache column 0 has no tiled pixels (untiled pad gap). "+
			"tileW=6, old startX=58 left x=0..57 blank. gridCachePad=%d", g.gridCachePad)
	}
}

// A tiled column carries the lattice lines somewhere in each period; an untiled one is fully transparent.
func gridColumnCovered(cache *ebiten.Image, x, tilePeriod int) bool {
	if tilePeriod < 1 {
		tilePeriod = 1
	}
	for y := 0; y < tilePeriod; y++ {
		if color.RGBAModel.Convert(cache.At(x, y)).(color.RGBA) != (color.RGBA{}) {
			return true
		}
	}
	return false
}

// Every pixel of the gap zone (x=0..57, y=0) must be tiled at minimum scale.
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
		if !gridColumnCovered(g.gridCache, x, tileW) {
			t.Fatalf("gridCache column %d has no tiled pixels (untiled gap). "+
				"tileW=%d gridCachePad=%d gapEnd=%d", x, tileW, g.gridCachePad, gapEnd)
		}
	}
}

// OffsetX=50 maximises the gap at scale 1 (startX would be 54).
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
	checkX := 53 // last column in old gap zone
	if !gridColumnCovered(g.gridCache, checkX, g.gridTile.Bounds().Dx()) {
		t.Fatalf("gridCache column %d has no tiled pixels (untiled pad gap). "+
			"tileW=60 phaseX=50 old startX=54 left x=0..53 blank. gridCachePad=%d",
			checkX, g.gridCachePad)
	}
}
