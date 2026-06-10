package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// ──────────────────────── Fix 2: visibleRows footer ──────────────────────────

// TestMobileVisibleRowsReservesFooter verifies that on mobile, visibleRows()
// subtracts one rowHeight for the "+" add-row footer — the add-row button is
// rendered as a full-width footer row (matching desktop), so its space must
// be reserved regardless of how many rows are present.
func TestMobileVisibleRowsReservesFooter(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	dv := g.drum
	rh := dv.rowHeight()
	area := dv.rowsAreaHeight()
	vis := dv.visibleRows()
	want := (area - rh) / rh
	if want < 0 {
		want = 0
	}
	// Guarantee-at-least-1 rule applies on mobile too.
	if want == 0 && area >= rh {
		want = 1
	}
	if vis != want {
		t.Fatalf("mobile visibleRows()=%d, want %d (rowsAreaHeight=%d, rowHeight=%d)", vis, want, area, rh)
	}
}

// TestDesktopVisibleRowsReservesFooter is a regression test ensuring desktop
// still subtracts one rowHeight for the "+" footer row.
func TestDesktopVisibleRowsReservesFooter(t *testing.T) {
	setupMobileTest(t, false) // desktop
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	dv := g.drum
	rh := dv.rowHeight()
	area := dv.rowsAreaHeight()
	vis := dv.visibleRows()
	// Desktop reserves one rowHeight for the "+" footer.
	want := (area - rh) / rh
	if want < 0 {
		want = 0
	}
	// Guarantee-at-least-1 rule may apply.
	if want == 0 && area >= rh {
		want = 1
	}
	if vis != want {
		t.Fatalf("desktop visibleRows()=%d, want %d (rowsAreaHeight=%d, rowHeight=%d)", vis, want, area, rh)
	}
}

// ──────────────────────── Fix 1: row divider suppression ─────────────────────

// TestMobileRowDividerNotDetectable verifies that detectRowDivider() always
// returns ("", -1) on mobile, preventing accidental row boundary drags.
func TestMobileRowDividerNotDetectable(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	dv := g.drum
	h := dv.layoutHandler
	// Sweep all Y positions across the drum view bounds.
	for y := dv.Bounds.Min.Y; y < dv.Bounds.Max.Y; y += 4 {
		midX := (dv.Bounds.Min.X + dv.Bounds.Max.X) / 2
		axis, idx := h.detectRowDivider(midX, y)
		if idx >= 0 {
			t.Fatalf("mobile detectRowDivider(%d, %d) = (%q, %d), want (\"\", -1)", midX, y, axis, idx)
		}
	}
}

// TestMobileLayoutGuidesSkipRowLines verifies that drawLayoutGuides() does not
// draw any horizontal row divider lines on mobile (column dividers are fine).
func TestMobileLayoutGuidesSkipRowLines(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	dv := g.drum
	// Intercept drawRect calls to detect horizontal lines.
	type drawnLine struct {
		r image.Rectangle
	}
	var lines []drawnLine
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		if filled {
			lines = append(lines, drawnLine{r: r})
		}
		orig(dst, r, c, filled)
	}
	defer func() { drawRect = orig }()

	dst := ebiten.NewImage(390, 844)
	dv.drawLayoutGuides(dst)

	// Row divider lines are horizontal (wider than tall). Column divider
	// lines are vertical (taller than wide). Check that no horizontal lines
	// were drawn — those are the row dividers we want suppressed.
	for _, l := range lines {
		if l.r.Dx() > l.r.Dy() {
			t.Fatalf("mobile drawLayoutGuides drew a horizontal line at %v — row dividers should be suppressed", l.r)
		}
	}
}

// ──────────────────────── Fix 3: scrollbar constrained ───────────────────────

// TestMobileScrollbarCoversRowsOnly verifies that the scrollbar track height
// is limited to vis*rowHeight — it should not extend into the leftover space
// below the last visible row.
func TestMobileScrollbarCoversRowsOnly(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	dv := g.drum
	// Add enough rows to make scrollbar meaningful.
	for i := 0; i < 10; i++ {
		dv.AddRow()
	}
	dv.refreshWidgetLayout()
	dv.recalcButtons()
	dv.calcLayout()

	vis := dv.visibleRows()
	rh := dv.rowHeight()
	maxTrackH := vis * rh
	barRect := dv.scrollBarRect()
	trackH := barRect.Dy()
	if trackH > maxTrackH {
		t.Fatalf("scrollbar track height %d exceeds vis*rowHeight %d (vis=%d, rh=%d)", trackH, maxTrackH, vis, rh)
	}
}

// TestMobileAddRowBtnDoesNotOverlapScrollTrack verifies that the full-width
// add-row footer button on mobile sits below the scrollable rows area, so it
// never overlaps the scrollbar track or any visible row.
func TestMobileAddRowBtnDoesNotOverlapScrollTrack(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	dv := g.drum
	// Add enough rows for scrollbar to appear.
	for i := 0; i < 10; i++ {
		dv.AddRow()
	}
	dv.refreshWidgetLayout()
	dv.recalcButtons()
	dv.calcLayout()

	btnRect := dv.addRowBtn().Rect()
	if btnRect.Empty() {
		t.Fatalf("mobile add-row button rect is empty; expected a visible footer button")
	}
	barRect := dv.scrollBarRect()
	if barRect.Empty() {
		t.Skip("scrollbar rect is empty")
	}
	// The button is full-width below the scrollable rows; it must not
	// vertically overlap the scrollbar track.
	if btnRect.Min.Y < barRect.Max.Y && btnRect.Max.Y > barRect.Min.Y {
		t.Fatalf("add-row button %v vertically overlaps scrollbar track %v", btnRect, barRect)
	}
}
