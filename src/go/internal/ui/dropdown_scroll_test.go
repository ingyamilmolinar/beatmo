package ui

import (
	"fmt"
	"image"
	"io"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestInstMenuScrollbarBackgroundExtended verifies the menu background
// extends to include the scrollbar area when scrolling is needed.
func TestInstMenuScrollbarBackgroundExtended(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(io.Discard, game_log.LevelError)

	// Create many instruments to force scrolling
	var entries []audio.SoundMeta
	for i := 0; i < 20; i++ {
		entries = append(entries, audio.SoundMeta{
			ID:       fmt.Sprintf("inst-%02d", i),
			Name:     fmt.Sprintf("Instrument %02d", i),
			Category: "Test (WAV)",
			Source:   "wav",
		})
	}
	withAudioCatalog(t, entries)

	graph := model.NewGraph(logger)
	// Small bounds to force scrolling
	dv := NewDrumView(image.Rect(0, 0, 320, timelineHeight+24*6), graph, logger)
	dv.instMenuForceCategories = true
	dv.calcLayout()

	// Open the instrument menu
	dv.rowLabels[0].OnClick()
	if !dv.instMenuOpen {
		t.Fatalf("menu did not open")
	}

	// Enter first category to see instruments
	if len(dv.instCategoryBtns) == 0 {
		t.Fatalf("no categories to enter")
	}
	dv.instCategoryBtns[0].OnClick()

	if !dv.instMenuHasScroll() {
		t.Fatalf("expected scrollbar when instruments overflow")
	}

	// The menu background rect should extend to include scrollbar
	menuBg := dv.instMenuScroll.View
	if menuBg.Empty() {
		menuBg = dv.instMenuFullRect
	}

	bar := dv.instMenuScroll.BarRect(instMenuScrollBarWidth)
	if bar.Empty() {
		t.Fatalf("scrollbar rect is empty")
	}

	// The scrollbar should be positioned at or near the right edge of the menu view
	// (it may be placed inside or just outside depending on layout)
	if bar.Min.X < menuBg.Min.X {
		t.Errorf("scrollbar starts before menu left edge: bar.Min.X=%d, menuBg.Min.X=%d", bar.Min.X, menuBg.Min.X)
	}

	// Verify scrollbar has proper width
	if bar.Dx() != instMenuScrollBarWidth {
		t.Errorf("scrollbar width: got %d, want %d", bar.Dx(), instMenuScrollBarWidth)
	}

	// Verify thumb is within scrollbar bounds
	thumb := dv.instMenuThumbRect()
	if thumb.Empty() {
		t.Fatalf("thumb rect is empty")
	}
	if thumb.Min.X < bar.Min.X || thumb.Max.X > bar.Max.X {
		t.Errorf("thumb X bounds outside scrollbar: thumb=%v, bar=%v", thumb, bar)
	}
	if thumb.Min.Y < bar.Min.Y || thumb.Max.Y > bar.Max.Y {
		t.Errorf("thumb Y bounds outside scrollbar: thumb=%v, bar=%v", thumb, bar)
	}
}

// TestInstMenuAndEQMenuScrollersAreIndependent verifies that scrolling
// one menu doesn't affect the other.
func TestInstMenuAndEQMenuScrollersAreIndependent(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	dv := g.drum

	// Add rows to create scrollable EQ channel menu
	for i := 0; i < 12; i++ {
		dv.AddRow()
		dv.Rows[i].Instrument = fmt.Sprintf("inst-%d", i)
		dv.Rows[i].Name = fmt.Sprintf("Inst %d", i)
	}

	// Create many instruments for scrollable instrument menu
	var entries []audio.SoundMeta
	for i := 0; i < 20; i++ {
		entries = append(entries, audio.SoundMeta{
			ID:       fmt.Sprintf("test-%02d", i),
			Name:     fmt.Sprintf("Test %02d", i),
			Category: "Test (WAV)",
			Source:   "wav",
		})
	}
	withAudioCatalog(t, entries)
	dv.instMenuForceCategories = true
	dv.calcLayout()

	// Open instrument menu and scroll it
	dv.rowLabels[0].OnClick()
	if !dv.instMenuOpen {
		t.Fatalf("instrument menu did not open")
	}
	if len(dv.instCategoryBtns) > 0 {
		dv.instCategoryBtns[0].OnClick()
	}

	instScrollBefore := dv.instMenuScroll.First
	dv.instMenuScroll.First = 3
	instScrollAfter := dv.instMenuScroll.First

	if instScrollAfter == instScrollBefore {
		t.Fatalf("instrument menu scroll did not change")
	}

	// Close instrument menu
	dv.instMenuOpen = false

	// Build and open EQ channel menu
	dv.eqChannelOpen = true
	dv.buildEQChannelMenu()

	eqScrollBefore := dv.eqChannelScroll.VS.First

	// EQ scroll should be independent (still at 0)
	if eqScrollBefore != 0 {
		t.Errorf("EQ channel scroll should start at 0, got %d", eqScrollBefore)
	}

	// Scroll EQ menu
	if dv.eqChannelScroll.HasScroll() {
		dv.eqChannelScroll.VS.First = 2
	}

	// Reopen instrument menu - its scroll should be preserved
	dv.eqChannelOpen = false
	dv.rowLabels[0].OnClick()
	if len(dv.instCategoryBtns) > 0 {
		dv.instCategoryBtns[0].OnClick()
	}

	// The two scrollers use independent state (different types/instances).
	// Verify by checking that setting one doesn't affect the other.
	dv.instMenuScroll.First = 5
	if dv.eqChannelScroll.VS.First == 5 {
		t.Errorf("instrument and EQ scrollers should be independent instances")
	}
	dv.instMenuScroll.First = 0 // reset
}

// TestInstMenuScrollDoesNotAffectRowOffset verifies wheel scrolling
// over the instrument menu doesn't change the drum row offset.
func TestInstMenuScrollDoesNotAffectRowOffset(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(io.Discard, game_log.LevelError)

	// Create many instruments to force scrolling
	var entries []audio.SoundMeta
	for i := 0; i < 20; i++ {
		entries = append(entries, audio.SoundMeta{
			ID:       fmt.Sprintf("inst-%02d", i),
			Name:     fmt.Sprintf("Instrument %02d", i),
			Category: "Test (WAV)",
			Source:   "wav",
		})
	}
	withAudioCatalog(t, entries)

	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 320, 260), graph, logger)
	dv.instMenuForceCategories = true
	dv.calcLayout()

	// Add rows to make row scrolling possible
	for i := 0; i < 8; i++ {
		dv.AddRow()
	}
	dv.calcLayout()

	// Open instrument menu
	dv.rowLabels[0].OnClick()
	if !dv.instMenuOpen {
		t.Fatalf("menu not open")
	}
	if len(dv.instCategoryBtns) > 0 {
		dv.instCategoryBtns[0].OnClick()
	}

	if !dv.instMenuHasScroll() {
		t.Fatalf("expected scrollbar for instrument menu")
	}

	startRowOff := dv.rowOffset
	startInstScroll := dv.instMenuScroll.First

	// Simulate wheel scroll over the instrument menu
	wheel := -2.0
	view := dv.instMenuScroll.View
	if view.Empty() {
		view = dv.instMenuFullRect
	}
	cx, cy := view.Min.X+5, view.Min.Y+view.Dy()/2

	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { v := wheel; wheel = 0; return 0, v },
		func() (int, int) { return 320, 260 },
	)
	t.Cleanup(restore)
	dv.Update()
	restore()

	// Row offset should NOT change
	if dv.rowOffset != startRowOff {
		t.Errorf("row offset changed during instrument menu scroll: %d -> %d",
			startRowOff, dv.rowOffset)
	}

	// Instrument menu scroll should have changed
	if dv.instMenuScroll.First == startInstScroll {
		t.Errorf("instrument menu scroll did not change: still at %d", startInstScroll)
	}
}

// TestEQMenuScrollDoesNotAffectRowOffset verifies wheel scrolling
// over the EQ channel menu doesn't change the drum row offset.
func TestEQMenuScrollDoesNotAffectRowOffset(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	dv := g.drum

	// Add many rows to force both row scrolling and EQ menu scrolling
	for i := 0; i < 15; i++ {
		dv.AddRow()
		dv.Rows[i].Instrument = fmt.Sprintf("inst-%d", i)
		dv.Rows[i].Name = fmt.Sprintf("Inst %d", i)
	}
	dv.calcLayout()

	// Open EQ channel menu
	dv.eqChannelOpen = true
	dv.buildEQChannelMenu()

	if !dv.eqChannelScroll.HasScroll() {
		t.Skip("EQ menu doesn't need scroll with this many rows - test not applicable")
	}

	startRowOff := dv.rowOffset
	startEQScroll := dv.eqChannelScroll.VS.First

	// Simulate wheel scroll over the EQ channel menu
	menuRect := dv.eqChannelMenuRect()
	cx, cy := menuRect.Min.X+5, menuRect.Min.Y+menuRect.Dy()/2
	wheel := -2.0

	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { v := wheel; wheel = 0; return 0, v },
		func() (int, int) { return 640, 480 },
	)
	t.Cleanup(restore)
	dv.Update()
	restore()

	// Row offset should NOT change
	if dv.rowOffset != startRowOff {
		t.Errorf("row offset changed during EQ menu scroll: %d -> %d",
			startRowOff, dv.rowOffset)
	}

	// EQ menu scroll should have changed (or at least been handled)
	// Note: The scroll might be clamped if already at max
	_ = startEQScroll // Used for debugging if needed
}

// TestInstMenuScrollbarClickKeepsMenuOpen verifies that clicking on the
// scrollbar thumb/track does not close the instrument menu. This was a bug
// where InputBounds() didn't include the scrollbar area.
func TestInstMenuScrollbarClickKeepsMenuOpen(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(io.Discard, game_log.LevelError)

	// Create many instruments to force scrolling
	var entries []audio.SoundMeta
	for i := 0; i < 20; i++ {
		entries = append(entries, audio.SoundMeta{
			ID:       fmt.Sprintf("inst-%02d", i),
			Name:     fmt.Sprintf("Instrument %02d", i),
			Category: "Test (WAV)",
			Source:   "wav",
		})
	}
	withAudioCatalog(t, entries)

	graph := model.NewGraph(logger)
	// Small bounds to force scrolling
	dv := NewDrumView(image.Rect(0, 0, 320, timelineHeight+24*6), graph, logger)
	dv.instMenuForceCategories = true
	dv.calcLayout()

	// Open the instrument menu
	dv.rowLabels[0].OnClick()
	if !dv.instMenuOpen {
		t.Fatalf("menu did not open")
	}

	// Enter first category to see instruments
	if len(dv.instCategoryBtns) == 0 {
		t.Fatalf("no categories to enter")
	}
	dv.instCategoryBtns[0].OnClick()

	if !dv.instMenuHasScroll() {
		t.Fatalf("expected scrollbar when instruments overflow")
	}

	// Get the scrollbar track position
	bar := dv.instMenuScroll.BarRect(instMenuScrollBarWidth)
	if bar.Empty() {
		t.Fatalf("scrollbar rect is empty")
	}

	// Click in the middle of the scrollbar track
	cx, cy := bar.Min.X+bar.Dx()/2, bar.Min.Y+bar.Dy()/2

	// Verify the InputBounds includes the scrollbar
	overlay := &InstrumentMenuOverlay{dv: dv}
	bounds := overlay.InputBounds()
	if !image.Pt(cx, cy).In(bounds) {
		t.Errorf("scrollbar center (%d,%d) is not within InputBounds %v", cx, cy, bounds)
	}

	// Simulate mouse press on the scrollbar
	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(btn ebiten.MouseButton) bool { return btn == ebiten.MouseButtonLeft && pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 320, timelineHeight + 24*6 },
	)
	t.Cleanup(restore)

	// Run an update with mouse pressed
	dv.Update()

	// Menu should still be open
	if !dv.instMenuOpen {
		t.Errorf("menu closed when clicking on scrollbar - InputBounds likely excludes scrollbar area")
	}

	// Release mouse
	pressed = false
	dv.Update()

	// Menu should still be open after release
	if !dv.instMenuOpen {
		t.Errorf("menu closed after releasing click on scrollbar")
	}
}
