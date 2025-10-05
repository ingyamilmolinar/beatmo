package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

// Ensure no horizontal baseline is drawn that would visually split a row into
// two cell bands. Previously a 1px line across the steps area made each row
// look like two stacked cell strips; that line should no longer be drawn.
func TestDrumViewRows_NoBaselineSplit(t *testing.T) {
	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 600, 220), nil, logger)
	// Add a second row
	dv.AddRow()
	dv.recalcButtons()
	dv.calcLayout()
	dst := ebiten.NewImage(600, 220)

	// Count 1px horizontal lines across the steps area using colTimelineBeat.
	// Expect zero to avoid visual splitting of rows.
	baseline := 0
	orig := drawRect
	drawRect = func(d *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		if filled {
			if r.Dy() == 1 && r.Min.X >= dv.timelineRect.Min.X && r.Max.X <= dv.timelineRect.Max.X {
				if color.RGBAModel.Convert(c).(color.RGBA) == colTimelineBeat {
					// Row-only (exclude the timeline header): start below timelineHeight
					if r.Min.Y >= dv.Bounds.Min.Y+timelineHeight {
						baseline++
					}
				}
			}
		}
		orig(d, r, c, filled)
	}
	defer func() { drawRect = orig }()

	dv.Draw(dst, map[int]int64{}, 0, nil, 0)
	if baseline != 0 {
		t.Fatalf("expected no row baseline lines, got %d", baseline)
	}
}
