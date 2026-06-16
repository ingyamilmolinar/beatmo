//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// newUpdateTestDV creates a DrumView suitable for Update() state transition tests.
// It ensures at least one row, computed layout, and clean parity state.
func newUpdateTestDV(t *testing.T) *DrumView {
	t.Helper()
	assertDefaultParityState(t)
	audio.ClearAllInsertEffects()
	audio.InitInsertChains(44100)
	dv := NewDrumView(ebiten.NewImage(800, 300).Bounds(), nil, game_log.New(nil, game_log.LevelError))
	dv.recalcButtons()
	dv.calcLayout()
	t.Cleanup(func() {
		audio.ClearAllInsertEffects()
	})
	if len(dv.Rows) == 0 {
		t.Fatal("newUpdateTestDV: expected at least one row")
	}
	return dv
}

// dvAdvance runs N Update() calls on the DrumView with no input active.
func dvAdvance(t *testing.T, dv *DrumView, n int) {
	t.Helper()
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 300 },
	)
	defer restore()
	for i := 0; i < n; i++ {
		dv.Update()
	}
}

// TestDrumViewUpdateSeqIncrements verifies that updateSeq increments on each
// Update() call.
func TestDrumViewUpdateSeqIncrements(t *testing.T) {
	dv := newUpdateTestDV(t)

	before := dv.updateSeq
	dvAdvance(t, dv, 2)
	after := dv.updateSeq

	if after != before+2 {
		t.Errorf("expected updateSeq to increment by 2: before=%d after=%d", before, after)
	}
}

// TestDrumViewUpdateEmptyRows verifies that Update() with no rows does not
// panic and returns early without incrementing updateSeq.
func TestDrumViewUpdateEmptyRows(t *testing.T) {
	dv := newUpdateTestDV(t)

	// Clear all rows to trigger the early return.
	dv.Rows = nil
	before := dv.updateSeq

	// Should not panic.
	dvAdvance(t, dv, 1)

	if dv.updateSeq != before {
		t.Errorf("expected updateSeq unchanged with empty rows: before=%d after=%d", before, dv.updateSeq)
	}
}

// TestDrumViewUpdatePopupBlocksInput verifies that when a popup (contextMenu)
// is open, mouse clicks on the timeline area do not start a drag.
func TestDrumViewUpdatePopupBlocksInput(t *testing.T) {
	dv := newUpdateTestDV(t)

	// Compute a position inside the steps area (timeline region).
	tlRect := dv.widgetRects[WidgetTimeline]
	if tlRect.Empty() {
		// Fallback: use the legacy stepsRect computation.
		tlRect.Min.X = dv.Bounds.Min.X + dv.labelW + dv.controlsW
		tlRect.Min.Y = dv.Bounds.Min.Y + dv.headerH
		tlRect.Max.X = dv.Bounds.Max.X
		tlRect.Max.Y = dv.Bounds.Max.Y - dv.eqH
	}
	cx := (tlRect.Min.X + tlRect.Max.X) / 2
	cy := (tlRect.Min.Y + dv.headerH + tlRect.Max.Y - dv.eqH) / 2
	if cy < tlRect.Min.Y+dv.headerH {
		cy = tlRect.Min.Y + dv.headerH + 5
	}

	// Open context menu to block input.
	dv.openContextMenuPortal()

	// Simulate a mouse press at the computed position.
	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 300 },
	)
	dv.Update()
	restore()

	if dv.dragging {
		t.Error("expected dragging=false when contextMenuOpen=true, but dragging was started")
	}
}

// TestDrumViewUpdateBPMDelta verifies that a pending bpmDelta is applied during
// Update() and then reset to zero.
func TestDrumViewUpdateBPMDelta(t *testing.T) {
	dv := newUpdateTestDV(t)

	dv.SetBPM(120)
	dv.bpmDelta = 10

	dvAdvance(t, dv, 1)

	if dv.bpm != 130 {
		t.Errorf("expected bpm=130 after delta=+10, got %d", dv.bpm)
	}
	if dv.bpmDelta != 0 {
		t.Errorf("expected bpmDelta=0 after Update(), got %d", dv.bpmDelta)
	}
}

// TestDrumViewUpdateLengthButtons verifies that setting lenIncPressed triggers
// a length increase during Update() and resets the flag.
func TestDrumViewUpdateLengthButtons(t *testing.T) {
	dv := newUpdateTestDV(t)

	origLen := dv.Length
	dv.lenIncPressed = true

	dvAdvance(t, dv, 1)

	// Length should increase by timelineUnitsPerBeat (at least 1).
	inc := max1(dv.timelineUnitsPerBeat)
	expected := dv.clampLength(origLen + inc)
	if dv.Length != expected {
		t.Errorf("expected Length=%d after lenIncPressed, got %d (origLen=%d, inc=%d)", expected, dv.Length, origLen, inc)
	}
	if dv.lenIncPressed {
		t.Error("expected lenIncPressed=false after Update(), but still true")
	}
}

// TestDrumViewUpdateDragging verifies that pressing the mouse inside the steps
// area starts dragging, and releasing the mouse stops it.
func TestDrumViewUpdateDragging(t *testing.T) {
	dv := newUpdateTestDV(t)

	// Compute a position inside the steps area.
	stepsMinX := dv.Bounds.Min.X + dv.labelW + dv.controlsW
	stepsMinY := dv.Bounds.Min.Y + dv.headerH
	stepsMaxX := dv.Bounds.Max.X
	stepsMaxY := dv.Bounds.Max.Y - dv.eqH
	// Use the widget rect if available.
	if tlRect, ok := dv.widgetRects[WidgetTimeline]; ok && !tlRect.Empty() {
		stepsMinX = tlRect.Min.X
		stepsMaxX = tlRect.Max.X
	}
	cx := (stepsMinX + stepsMaxX) / 2
	cy := (stepsMinY + stepsMaxY) / 2
	if cy <= stepsMinY || cy >= stepsMaxY {
		cy = stepsMinY + 5
	}

	// Ensure no popups are open.
	dv.CloseAllPopups()
	dv.closeVolumePopup()
	dv.closeMasterVolumePopup()

	// Step 1: Press mouse inside the steps area.
	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 300 },
	)
	dv.Update()
	restore()

	// Check drag state: zone-based when TimelineZone exists, legacy field otherwise.
	isDragging := dv.dragging
	if dv.timelineZone != nil {
		isDragging = dv.timelineZone.IsDragging()
	}
	if !isDragging {
		t.Fatal("expected dragging=true after mouse press inside steps area")
	}

	// Step 2: Release mouse.
	restore = SetInputForTest(
		func() (int, int) { return cx, cy },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 300 },
	)
	dv.Update()
	restore()

	isDragging = dv.dragging
	if dv.timelineZone != nil {
		isDragging = dv.timelineZone.IsDragging()
	}
	if isDragging {
		t.Error("expected dragging=false after mouse release, but still dragging")
	}
}

// TestDrumViewUpdateImportTimeout verifies that a stale import flag is
// automatically cleared after 600 frames (~10s at 60fps).
func TestDrumViewUpdateImportTimeout(t *testing.T) {
	dv := newUpdateTestDV(t)

	dv.importing = true
	dv.importCh = make(chan importResult, 1) // leave empty so select hits default
	dv.importAttemptFrame = 0
	dv.importAttemptUpdate = 0
	dv.frame = 0

	// Advance 601 frames — should trigger the timeout guard.
	for i := 0; i < 601; i++ {
		dv.frame++
		dvAdvance(t, dv, 1)
		if !dv.importing {
			break
		}
	}

	if dv.importing {
		t.Error("expected importing=false after 600+ frames timeout")
	}
}

// TestDrumViewUpdateBPMBoxEnterCommit verifies that typing a valid BPM
// in the shared editor and pressing Enter commits the new BPM value.
func TestDrumViewUpdateBPMBoxEnterCommit(t *testing.T) {
	dv := newUpdateTestDV(t)
	dv.SetBPM(120)

	// Open the shared editor and set its text.
	dv.transportZone.openBPMEditor()
	dv.transportZone.paramEditor.ti.SetText("150")

	// Simulate Enter key press — the editor commits on Enter inside Update().
	enterPressed := true
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == ebiten.KeyEnter && enterPressed },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 300 },
	)
	dv.Update()
	restore()
	enterPressed = false

	if dv.bpm != 150 {
		t.Errorf("expected BPM=150 after Enter commit, got %d", dv.bpm)
	}
	if dv.transportZone.paramEditor.Active() {
		t.Error("expected editor to be inactive after Enter commit")
	}
}

// TestDrumViewUpdateBPMBoxInvalidInput verifies that committing an invalid
// BPM string flashes the editor error and leaves BPM unchanged.
func TestDrumViewUpdateBPMBoxInvalidInput(t *testing.T) {
	dv := newUpdateTestDV(t)
	dv.SetBPM(120)

	dv.transportZone.openBPMEditor()
	dv.transportZone.paramEditor.ti.SetText("abc")
	dv.transportZone.paramEditor.commit()

	if dv.transportZone.paramEditor.errorAnim <= 0 {
		t.Error("expected editor errorAnim > 0 after invalid BPM input")
	}
	if dv.transportZone.BPM() != 120 {
		t.Errorf("expected BPM unchanged at 120 after invalid input, got %d", dv.transportZone.BPM())
	}
}

// TestDrumViewUpdateBPMBoxEmptyInput verifies that an empty editor commit is
// invalid and leaves BPM unchanged (no write).
func TestDrumViewUpdateBPMBoxEmptyInput(t *testing.T) {
	dv := newUpdateTestDV(t)
	dv.SetBPM(90)

	dv.transportZone.openBPMEditor()
	dv.transportZone.paramEditor.ti.SetText("")
	dv.transportZone.paramEditor.commit()

	if dv.transportZone.BPM() != 90 {
		t.Errorf("expected BPM unchanged at 90 on empty input, got %d", dv.transportZone.BPM())
	}
}

// TestDrumViewUpdateSliderIsolationRelease verifies that the tree's capture
// mechanism releases slider groups on mouse release. Slider dispatch now goes
// through the zone tree's hit areas (TransportZone for main volume).
func TestDrumViewUpdateSliderIsolationRelease(t *testing.T) {
	dv := newUpdateTestDV(t)

	sl := dv.mainVolSlider()
	if sl == nil {
		t.Skip("mainVolSlider nil in test DrumView")
	}
	r := sl.Rect()
	if r.Empty() {
		t.Skip("mainVolSlider rect empty")
	}

	// Press inside the slider via the tree dispatch.
	cx, cy := r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2
	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 300 },
	)
	dv.Update()
	restore()

	if dv.mainVolGroup() == nil {
		t.Skip("mainVolGroup nil")
	}

	// Release mouse via the tree dispatch.
	restore = SetInputForTest(
		func() (int, int) { return cx, cy },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 300 },
	)
	dv.Update()
	restore()

	if dv.mainVolGroup().Capturing() {
		t.Errorf("expected mainVolGroup not capturing after release")
	}
}

// TestDrumViewUpdatePopupBlocksAll verifies that each popup flag causes
// Update() to return early (no drag started).
func TestDrumViewUpdatePopupBlocksAll(t *testing.T) {
	popupFlags := []struct {
		name   string
		setter func(*DrumView)
	}{
		{"contextMenuOpen", func(dv *DrumView) { dv.openContextMenuPortal() }},
		{"overflowMenuOpen", func(dv *DrumView) { dv.openOverflowMenuPortal() }},
		{"volPopupOpen", func(dv *DrumView) {
			dv.volPopup.Open(image.Rect(100, 100, 130, 120), dv.Bounds, dv.headerH)
			dv.openVolPopupPortal()
		}},
		{"fxPanelOpen", func(dv *DrumView) { dv.openFXPanelPortal() }},
		{"masterVolPopupOpen", func(dv *DrumView) {
			dv.masterVolPopup.Open(image.Rect(100, 100, 130, 120), dv.Bounds, dv.headerH)
			dv.openMasterVolPopupPortal()
		}},
	}

	for _, tc := range popupFlags {
		t.Run(tc.name, func(t *testing.T) {
			dv := newUpdateTestDV(t)

			// Compute steps area center.
			stepsMinX := dv.Bounds.Min.X + dv.labelW + dv.controlsW
			stepsMinY := dv.Bounds.Min.Y + dv.headerH
			stepsMaxX := dv.Bounds.Max.X
			stepsMaxY := dv.Bounds.Max.Y - dv.eqH
			cx := (stepsMinX + stepsMaxX) / 2
			cy := (stepsMinY + stepsMaxY) / 2
			if cy <= stepsMinY || cy >= stepsMaxY {
				cy = stepsMinY + 5
			}

			tc.setter(dv)

			restore := SetInputForTest(
				func() (int, int) { return cx, cy },
				func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
				func(ebiten.Key) bool { return false },
				func() []rune { return nil },
				func() (float64, float64) { return 0, 0 },
				func() (int, int) { return 800, 300 },
			)
			dv.Update()
			restore()

			if dv.dragging {
				t.Errorf("expected dragging=false when %s=true", tc.name)
			}
		})
	}
}

// TestDrumViewUpdateBPMBoxBlurCommit verifies that opening the shared editor,
// entering a value, then clicking outside commits the BPM on focus loss.
func TestDrumViewUpdateBPMBoxBlurCommit(t *testing.T) {
	dv := newUpdateTestDV(t)
	dv.SetBPM(120)

	// Open the editor and type a value.
	dv.transportZone.openBPMEditor()
	dv.transportZone.paramEditor.ti.SetText("90")

	// Now simulate blur by clicking outside the editor's box rect.
	editRect := dv.transportZone.paramEditor.ti.Rect
	outsideX := editRect.Max.X + 50
	outsideY := editRect.Max.Y + 50

	restore := SetInputForTest(
		func() (int, int) { return outsideX, outsideY },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 300 },
	)
	dv.Update()
	restore()

	if dv.bpm != 90 {
		t.Errorf("expected BPM=90 after blur commit, got %d", dv.bpm)
	}
}
