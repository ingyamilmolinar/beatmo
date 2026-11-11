//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// drawnRectRecord captures a drawRect call for test inspection.
type drawnRectRecord struct {
	Rect   image.Rectangle
	Color  color.Color
	Filled bool
}

// newZeroOriginDrumView creates a DrumView with bounds starting at (0,0).
func newZeroOriginDrumView(w, h int) *DrumView {
	logger := game_log.New(nil, game_log.LevelError)
	g := model.NewGraph(logger)
	bounds := image.Rect(0, 0, w, h)
	dv := NewDrumView(bounds, g, logger)
	dv.widgets.SetBounds(bounds)
	dv.refreshWidgetLayout()
	return dv
}

// TestColumnDividerLineRespectsWidgetSpan verifies that the column divider line
// is NOT drawn through the Wave widget row (row 2), which spans both columns.
func TestColumnDividerLineRespectsWidgetSpan(t *testing.T) {
	dv := newZeroOriginDrumView(800, 400)
	if dv.widgets == nil {
		t.Fatal("widgets board not initialized")
	}

	var rects []drawnRectRecord
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		rects = append(rects, drawnRectRecord{Rect: r, Color: c, Filled: filled})
		orig(dst, r, c, filled)
	}
	defer func() { drawRect = orig }()

	img := ebiten.NewImage(dv.Bounds.Dx(), dv.Bounds.Dy())
	dv.drawLayoutGuides(img)

	if len(dv.widgets.colPos) < 3 {
		t.Fatal("expected at least 3 column positions (2 columns)")
	}

	colDivX := dv.widgets.colPos[1]
	waveY0 := dv.widgets.rowPos[2]

	t.Logf("Column divider X=%d, Wave Y start=%d", colDivX, waveY0)

	// A violating rect is a vertical line that starts ABOVE waveY0 and extends past it.
	// This catches the full-height column divider line but not pill handle rects
	// (which start near the vertical center, well below waveY0).
	for _, r := range rects {
		cx := (r.Rect.Min.X + r.Rect.Max.X) / 2
		if cx < colDivX-3 || cx > colDivX+3 {
			continue
		}
		if r.Rect.Min.Y < waveY0 && r.Rect.Max.Y > waveY0 {
			t.Errorf("Column divider line crosses into Wave widget area: rect=%v, wave starts at Y=%d",
				r.Rect, waveY0)
		}
	}
}

// TestRowDivider1_2LineSuppressed verifies that no horizontal divider line
// is drawn at the row 1/2 boundary because WidgetWave at row 2 is full-width.
func TestRowDivider1_2LineSuppressed(t *testing.T) {
	dv := newZeroOriginDrumView(800, 400)
	if dv.widgets == nil {
		t.Fatal("widgets board not initialized")
	}

	var rects []drawnRectRecord
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		rects = append(rects, drawnRectRecord{Rect: r, Color: c, Filled: filled})
		orig(dst, r, c, filled)
	}
	defer func() { drawRect = orig }()

	img := ebiten.NewImage(dv.Bounds.Dx(), dv.Bounds.Dy())
	dv.drawLayoutGuides(img)

	row2Y := dv.widgets.rowPos[2]
	t.Logf("Row 1/2 boundary Y=%d", row2Y)

	for _, r := range rects {
		cy := (r.Rect.Min.Y + r.Rect.Max.Y) / 2
		// Check for horizontal lines centered on the row 1/2 boundary.
		if cy >= row2Y-3 && cy <= row2Y+3 {
			// Ensure it's a horizontal line (wide, thin).
			w := r.Rect.Dx()
			h := r.Rect.Dy()
			if w > h*3 {
				t.Errorf("Found horizontal divider line at row 1/2 boundary: rect=%v", r.Rect)
			}
		}
	}
}

// TestRowDividerLineRespectsWidgetSpan verifies that the row divider line
// between row 0 and row 1 is NOT drawn through the Timeline column,
// which spans rows 0-1.
func TestRowDividerLineRespectsWidgetSpan(t *testing.T) {
	dv := newZeroOriginDrumView(800, 400)
	if dv.widgets == nil {
		t.Fatal("widgets board not initialized")
	}

	var rects []drawnRectRecord
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		rects = append(rects, drawnRectRecord{Rect: r, Color: c, Filled: filled})
		orig(dst, r, c, filled)
	}
	defer func() { drawRect = orig }()

	img := ebiten.NewImage(dv.Bounds.Dx(), dv.Bounds.Dy())
	dv.drawLayoutGuides(img)

	if len(dv.widgets.rowPos) < 3 {
		t.Fatal("expected at least 3 row positions")
	}

	rowDivY := dv.widgets.rowPos[1]
	timelineX0 := dv.widgets.colPos[1]

	t.Logf("Row divider Y=%d, Timeline starts at X=%d", rowDivY, timelineX0)

	// A violating rect is a horizontal line that starts BEFORE timelineX0 and extends past it.
	// This catches the full-width row divider line but not pill handle rects
	// (which start near the horizontal center, well past timelineX0).
	for _, r := range rects {
		cy := (r.Rect.Min.Y + r.Rect.Max.Y) / 2
		if cy < rowDivY-3 || cy > rowDivY+3 {
			continue
		}
		if r.Rect.Min.X < timelineX0 && r.Rect.Max.X > timelineX0 {
			t.Errorf("Row divider line crosses into Timeline widget area: rect=%v, timeline starts at X=%d",
				r.Rect, timelineX0)
		}
	}
}
