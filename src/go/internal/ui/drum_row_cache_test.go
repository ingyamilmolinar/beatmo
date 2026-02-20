package ui

import (
	"fmt"
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestRowCacheRebuildOnLengthAndColor verifies that the per-row sprite cache is
// reused when stable and rebuilt on length or color changes. With image-pool
// optimization, the image pointer may be reused across rebuilds, so we track
// rebuilds via rowCacheGen (incremented on every full rebuild).
func TestRowCacheRebuildOnLengthAndColor(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), nil, logger)
	dv.recalcButtons()
	dv.calcLayout()
	dst := ebiten.NewImage(400, 200)
	// Initial draw builds caches
	dv.Draw(dst, nil, 0, nil, 0)
	if len(dv.rowCache) == 0 || dv.rowCache[0] == nil {
		t.Fatalf("expected row cache to be built")
	}
	genAfterFirst := dv.rowCacheGen[0]
	// Second draw should reuse (gen unchanged)
	dv.Draw(dst, nil, 0, nil, 0)
	if dv.rowCacheGen[0] != genAfterFirst {
		t.Fatalf("expected row cache reuse; gen changed from %d to %d on second draw", genAfterFirst, dv.rowCacheGen[0])
	}
	// Change length -> rebuild (gen incremented)
	pressLenInc(t, dv)
	dv.Update()
	dv.Draw(dst, nil, 0, nil, 0)
	if dv.rowCacheGen[0] <= genAfterFirst {
		t.Fatalf("expected row cache rebuild after length change; gen %d unchanged", dv.rowCacheGen[0])
	}
	// Change color -> rebuild (gen incremented again)
	genBeforeColor := dv.rowCacheGen[0]
	dv.SetRowColor(0, color.RGBA{255, 0, 0, 255})
	dv.Draw(dst, nil, 0, nil, 0)
	if dv.rowCacheGen[0] <= genBeforeColor {
		t.Fatalf("expected row cache rebuild after color change; gen %d unchanged", dv.rowCacheGen[0])
	}
}

// helper: convert hex string back to RGBA for test convenience.
func colorKeyToRGBA(hex string) (c image.Uniform) {
	// hex is RRGGBBAA
	var r, g, b, a uint8
	if _, err := fmt.Sscanf(hex, "%02X%02X%02X%02X", &r, &g, &b, &a); err == nil {
		return image.Uniform{C: color.RGBA{r, g, b, a}}
	}
	return image.Uniform{C: color.RGBA{255, 0, 0, 255}}
}
