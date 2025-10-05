package ui

import (
	"os"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

// TestGridLayerCacheReuse ensures the grid layer cache reuses via blit when the
// camera pans within the pad, and only rebuilds on first draw.
func TestGridLayerCacheReuse(t *testing.T) {
	logger := game_log.New(os.Stdout, game_log.LevelError)
	g := New(logger)
	g.Layout(640, 360)
	// Force a known scale to stabilize stepPx.
	g.cam.Scale = 1.0
	g.cam.Snap()

	dst := ebiten.NewImage(640, g.split.Y)
	// Make edge cache pad distinct to identify grid blits by offset.
	g.edgeCachePad = 7
	// Intercept blitCache to record offsets.
	type call struct{ dx, dy int }
	var blits []call
	orig := blitCache
	blitCache = func(dst, src *ebiten.Image, dx, dy int) {
		blits = append(blits, call{dx, dy})
		orig(dst, src, dx, dy)
	}
	defer func() { blitCache = orig }()

	// First draw builds the cache; capture a grid-targeting blit (dx== -gridPad).
	g.drawGridPane(dst)
	gridPad := -g.gridCachePad
	found := false
	for _, c := range blits {
		if c.dx == gridPad && c.dy == gridPad {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected grid blit with dx=dy=%d among %d calls", gridPad, len(blits))
	}
	if g.gridCache == nil || g.gridCacheW == 0 || g.gridCacheH == 0 {
		t.Fatalf("grid cache not built on first draw")
	}
	// Small pan inside pad; expect reuse blit with delta applied and same cache pointer.
	oldCache := g.gridCache
	blits = blits[:0]
	g.cam.OffsetX += 10
	g.cam.OffsetY += 6
	g.cam.Snap()
	g.drawGridPane(dst)
	if g.gridCache != oldCache {
		t.Fatalf("expected grid cache reuse, but cache image was rebuilt")
	}
	// Find grid-targeting blit for the reuse pass: dx/dy should shift by the pan.
	// Expected dx,dy = -gridPad + delta (10,6)
	wantDX, wantDY := gridPad+10, gridPad+6
	found = false
	for _, c := range blits {
		if c.dx == wantDX && c.dy == wantDY {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected reuse grid blit with dx=%d dy=%d not found; calls=%v", wantDX, wantDY, blits)
	}
}
