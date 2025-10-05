package ui

import (
	"fmt"
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

// TestRowCacheRebuildOnLengthAndColor verifies that the per-row sprite cache is
// reused when stable and rebuilt on length or color changes.
func TestRowCacheRebuildOnLengthAndColor(t *testing.T) {
	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), nil, logger)
	dv.recalcButtons()
	dv.calcLayout()
	dst := ebiten.NewImage(400, 200)
	// Initial draw builds caches
	dv.Draw(dst, map[int]int64{}, 0, nil, 0)
	if len(dv.rowCache) == 0 || dv.rowCache[0] == nil {
		t.Fatalf("expected row cache to be built")
	}
	first := dv.rowCache[0]
	// Second draw should reuse
	dv.Draw(dst, map[int]int64{}, 0, nil, 0)
	if dv.rowCache[0] != first {
		t.Fatalf("expected row cache reuse; got new pointer on second draw")
	}
	// Change length -> rebuild
	dv.lenIncPressed = true
	dv.Update()
	dv.Draw(dst, map[int]int64{}, 0, nil, 0)
	if dv.rowCache[0] == first {
		t.Fatalf("expected row cache rebuild after length change")
	}
	// Change color -> rebuild
	prev := dv.rowCache[0]
	dv.SetRowColor(0, color.RGBA{255, 0, 0, 255})
	dv.Draw(dst, map[int]int64{}, 0, nil, 0)
	if dv.rowCache[0] == prev {
		t.Fatalf("expected row cache rebuild after color change")
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
