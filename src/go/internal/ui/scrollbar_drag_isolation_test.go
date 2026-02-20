//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestScrollbarDragDoesNotActivateRowButtons verifies that when the scrollbar
// thumb is being dragged and the cursor moves over row buttons (mute, solo,
// delete, etc.), those buttons do not fire.
func TestScrollbarDragDoesNotActivateRowButtons(t *testing.T) {
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 300, 800, 600), graph, logger)
	dv.recalcButtons()

	// Verify baseline: row 0 is not muted
	if len(dv.Rows) == 0 {
		t.Fatal("DrumView has no rows")
	}
	if dv.Rows[0].Muted {
		t.Fatal("row 0 should start unmuted")
	}

	// Simulate scrollbar drag active
	dv.rowScroll().VS.dragging = true
	muteRect := dv.rowMuteBtns()[0].Rect()
	cx := muteRect.Min.X + muteRect.Dx()/2
	cy := muteRect.Min.Y + muteRect.Dy()/2

	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	dv.Update()

	if dv.Rows[0].Muted {
		t.Error("mute button fired during scrollbar drag — expected it to be blocked")
	}
}

// TestTouchScrollDoesNotActivateRowButtons verifies that while a touch scroll
// is active, row buttons don't fire.
func TestTouchScrollDoesNotActivateRowButtons(t *testing.T) {
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 300, 800, 600), graph, logger)
	dv.recalcButtons()

	if len(dv.Rows) == 0 {
		t.Fatal("DrumView has no rows")
	}

	// Start a touch scroll and move past dead zone to commit vertical direction
	dv.rowScroll().HandleTouchBegin(100, 400)
	dv.rowScroll().TS.Move(100, 420) // 20px vertical > 8px dead zone

	muteRect := dv.rowMuteBtns()[0].Rect()
	cx := muteRect.Min.X + muteRect.Dx()/2
	cy := muteRect.Min.Y + muteRect.Dy()/2

	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	dv.Update()

	if dv.Rows[0].Muted {
		t.Error("mute button fired during touch scroll — expected it to be blocked")
	}
}

// TestScrubbingDoesNotActivateRowButtons verifies that while scrubbing the
// timeline, row buttons don't fire.
func TestScrubbingDoesNotActivateRowButtons(t *testing.T) {
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 300, 800, 600), graph, logger)
	dv.recalcButtons()

	if len(dv.Rows) == 0 {
		t.Fatal("DrumView has no rows")
	}

	dv.scrubbing = true

	muteRect := dv.rowMuteBtns()[0].Rect()
	cx := muteRect.Min.X + muteRect.Dx()/2
	cy := muteRect.Min.Y + muteRect.Dy()/2

	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	dv.Update()

	if dv.Rows[0].Muted {
		t.Error("mute button fired during scrubbing — expected it to be blocked")
	}
}

// TestAnyDragActiveIncludesAllStates verifies that anyDragActive() returns true
// for each individual drag state and false when all are inactive.
func TestAnyDragActiveIncludesAllStates(t *testing.T) {
	dv := &DrumView{
		rowRackZone:     NewRowRackZone(RowRackCallbacks{}),
		eqCurveDragBand: -1,
	}
	dv.transportZone = &TransportZone{}

	if dv.anyDragActive() {
		t.Error("anyDragActive() should be false when all states are inactive")
	}

	tests := []struct {
		name  string
		setup func()
		reset func()
	}{
		{
			"rowScroll.Dragging",
			func() { dv.rowScroll().VS.dragging = true },
			func() { dv.rowScroll().VS.dragging = false },
		},
		{
			"dragging",
			func() { dv.dragging = true },
			func() { dv.dragging = false },
		},
		{
			"scrubbing",
			func() { dv.scrubbing = true },
			func() { dv.scrubbing = false },
		},
		{
			"rowVolGroup.Capturing",
			func() {
				// Set up a standalone slider group that captures without
				// the popup-opening onChange callback.
				s := NewSlider(0.5)
				s.SetRect(image.Rect(0, 0, 100, 20))
				s.dragging = true // directly set capture state
				grp := NewSliderGroup([]*Slider{s}, nil)
				grp.HandleInput(50, 10, true) // activates the group
				dv.rowRackZone.rowVolGroup = grp
			},
			func() {
				dv.rowVolGroup().Release()
				dv.rowVolGroup().SetSliders(nil)
			},
		},
		{
			"rowScroll.ScrollingCommitted",
			func() { dv.rowScroll().HandleTouchBegin(0, 0); dv.rowScroll().TS.Move(0, 20) },
			func() { dv.rowScroll().ResetTouch() },
		},
		{
			"instMenuScroll.dragging",
			func() { dv.instMenuScroll.dragging = true },
			func() { dv.instMenuScroll.dragging = false },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.setup()
			if !dv.anyDragActive() {
				t.Errorf("anyDragActive() should be true when %s is set", tc.name)
			}
			tc.reset()
			if dv.anyDragActive() {
				t.Errorf("anyDragActive() should be false after resetting %s", tc.name)
			}
		})
	}
}
