//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// --- helpers ---

// newFullDrumView creates a fully initialized DrumView suitable for testing
// open* portal functions that capture closures referencing DrumView internals.
func newFullDrumView(t *testing.T) *DrumView {
	t.Helper()
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)
	dv.calcLayout()
	return dv
}

// newTestSliderPopup creates a SliderPopup opened with a valid rect for testing.
func newTestSliderPopup() *SliderPopup {
	sp := NewSliderPopup(SliderPopupConfig{
		ID:       "test",
		GetValue: func() float64 { return 0.5 },
		SetValue: func(float64) {},
	})
	sp.Open(image.Rect(100, 100, 140, 130), image.Rect(0, 0, 800, 600), 40)
	return sp
}

// ============================================================
// openOverflowMenuPortal
// ============================================================

func TestOpenOverflowMenuPortal(t *testing.T) {
	dv := newFullDrumView(t)

	dv.openOverflowMenuPortal()

	if !dv.portal().Has("overflow-menu") {
		t.Error("portal 'overflow-menu' should be open")
	}
}

func TestOpenOverflowMenuPortal_NilTree(t *testing.T) {
	dv := &DrumView{}
	dv.openOverflowMenuPortal() // should not panic
}

func TestOpenOverflowMenuPortal_OnClose(t *testing.T) {
	dv := newFullDrumView(t)

	dv.openOverflowMenuPortal()
	if !dv.portal().Has("overflow-menu") {
		t.Fatal("portal 'overflow-menu' should be open")
	}

	// Close triggers OnClose callback (cancels deferred tap, clears file picker rects).
	dv.portal().Close("overflow-menu")

	if dv.portal().Has("overflow-menu") {
		t.Error("portal 'overflow-menu' should be closed")
	}
	if dv.overflowMenuScroll != nil && dv.overflowMenuScroll.TapActive() {
		t.Error("overflow menu deferred tap should be cancelled after OnClose")
	}
}

func TestOpenOverflowMenuPortal_Idempotent(t *testing.T) {
	dv := newFullDrumView(t)

	dv.openOverflowMenuPortal()
	dv.openOverflowMenuPortal() // second call should replace, not duplicate

	if dv.portal().StackLen() != 1 {
		t.Errorf("expected 1 portal entry, got %d", dv.portal().StackLen())
	}
}

// ============================================================
// openContextMenuPortal
// ============================================================

func TestOpenContextMenuPortal(t *testing.T) {
	dv := newFullDrumView(t)

	dv.openContextMenuPortal()

	if !dv.portal().Has("context-menu") {
		t.Error("portal 'context-menu' should be open")
	}
}

func TestOpenContextMenuPortal_NilTree(t *testing.T) {
	dv := &DrumView{}
	dv.openContextMenuPortal() // should not panic
}

func TestOpenContextMenuPortal_CloseRoundtrip(t *testing.T) {
	dv := newFullDrumView(t)

	dv.openContextMenuPortal()
	if !dv.portal().Has("context-menu") {
		t.Fatal("portal 'context-menu' should be open")
	}

	dv.portal().Close("context-menu")

	if dv.portal().Has("context-menu") {
		t.Error("portal 'context-menu' should be closed")
	}
}

// ============================================================
// openFXPanelPortal
// ============================================================

func TestOpenFXPanelPortal(t *testing.T) {
	dv := newFullDrumView(t)

	dv.openFXPanelPortal()

	if !dv.portal().Has("fx-panel") {
		t.Error("portal 'fx-panel' should be open")
	}
}

func TestOpenFXPanelPortal_NilTree(t *testing.T) {
	dv := &DrumView{}
	dv.openFXPanelPortal() // should not panic
}

func TestOpenFXPanelPortal_OnClose(t *testing.T) {
	dv := newFullDrumView(t)

	// Set state that OnClose should clean up.
	dv.fxAddMenuOpen = true
	dv.fxPanelBtns = []*Button{{Text: "x"}}
	dv.fxPanelSliders = []*Slider{{}}
	dv.fxSliderDragging = true
	dv.fxScrollOffsetPx = 42
	dv.fxScrollMaxPx = 100
	dv.fxPanelRect = image.Rect(10, 10, 200, 200)

	dv.openFXPanelPortal()

	// Close triggers OnClose.
	dv.portal().Close("fx-panel")

	if dv.fxAddMenuOpen {
		t.Error("fxAddMenuOpen should be false after OnClose")
	}
	if dv.fxPanelBtns != nil {
		t.Error("fxPanelBtns should be nil after OnClose")
	}
	if dv.fxPanelSliders != nil {
		t.Error("fxPanelSliders should be nil after OnClose")
	}
	if dv.fxSliderDragging {
		t.Error("fxSliderDragging should be false after OnClose")
	}
	if dv.fxScrollOffsetPx != 0 {
		t.Errorf("fxScrollOffsetPx should be 0, got %d", dv.fxScrollOffsetPx)
	}
	if dv.fxScrollMaxPx != 0 {
		t.Errorf("fxScrollMaxPx should be 0, got %d", dv.fxScrollMaxPx)
	}
	if !dv.fxPanelRect.Empty() {
		t.Errorf("fxPanelRect should be empty, got %v", dv.fxPanelRect)
	}
}

// ============================================================
// openNamingPortal
// ============================================================

func TestOpenNamingPortal(t *testing.T) {
	dv := newFullDrumView(t)

	dv.openNamingPortal()

	if !dv.portal().Has("naming") {
		t.Error("portal 'naming' should be open")
	}
}

func TestOpenNamingPortal_NilTree(t *testing.T) {
	dv := &DrumView{}
	dv.openNamingPortal() // should not panic
}

func TestOpenNamingPortal_Modal(t *testing.T) {
	dv := newFullDrumView(t)

	dv.openNamingPortal()

	if !dv.portal().HasModal() {
		t.Error("naming portal should be modal")
	}
}

func TestOpenNamingPortal_OnClose(t *testing.T) {
	dv := newFullDrumView(t)

	// Set state that OnClose should clean up.
	dv.pendingWAV = "test.wav"
	dv.nameInput = "TestInstrument"
	dv.nameBox = NewTextInput(image.Rect(10, 10, 200, 40), BPMBoxStyle)

	dv.openNamingPortal()

	// Close triggers OnClose.
	dv.portal().Close("naming")

	if dv.pendingWAV != "" {
		t.Errorf("pendingWAV should be empty, got %q", dv.pendingWAV)
	}
	if dv.nameInput != "" {
		t.Errorf("nameInput should be empty, got %q", dv.nameInput)
	}
	if dv.nameBox != nil {
		t.Error("nameBox should be nil after OnClose")
	}
}

// ============================================================
// openVolPopupPortal
// ============================================================

func TestOpenVolPopupPortal(t *testing.T) {
	dv, restore := setupPortalCloseTest(t)
	defer restore()

	dv.volPopup = newTestSliderPopup()
	dv.openVolPopupPortal()

	if !dv.portal().Has("volume-popup") {
		t.Error("portal 'volume-popup' should be open")
	}
}

func TestOpenVolPopupPortal_NilTree(t *testing.T) {
	dv := &DrumView{}
	dv.volPopup = newTestSliderPopup()
	dv.openVolPopupPortal() // should not panic
}

func TestOpenVolPopupPortal_NilPopup(t *testing.T) {
	dv, restore := setupPortalCloseTest(t)
	defer restore()

	dv.openVolPopupPortal() // volPopup is nil, should not panic

	if dv.portal().Has("volume-popup") {
		t.Error("portal 'volume-popup' should NOT be open when volPopup is nil")
	}
}

func TestOpenVolPopupPortal_Modal(t *testing.T) {
	dv, restore := setupPortalCloseTest(t)
	defer restore()

	dv.volPopup = newTestSliderPopup()
	dv.openVolPopupPortal()

	if !dv.portal().HasModal() {
		t.Error("volume-popup portal should be modal")
	}
}

func TestOpenVolPopupPortal_OnClose(t *testing.T) {
	dv, restore := setupPortalCloseTest(t)
	defer restore()

	dv.volPopup = newTestSliderPopup()
	if !dv.volPopup.IsOpen() {
		t.Fatal("volPopup should be open before portal open")
	}

	dv.openVolPopupPortal()
	dv.portal().Close("volume-popup")

	if dv.volPopup.IsOpen() {
		t.Error("volPopup should be closed after OnClose")
	}
}

// ============================================================
// openMasterVolPopupPortal
// ============================================================

func TestOpenMasterVolPopupPortal(t *testing.T) {
	dv, restore := setupPortalCloseTest(t)
	defer restore()

	dv.masterVolPopup = newTestSliderPopup()
	dv.openMasterVolPopupPortal()

	if !dv.portal().Has("master-volume-popup") {
		t.Error("portal 'master-volume-popup' should be open")
	}
}

func TestOpenMasterVolPopupPortal_NilTree(t *testing.T) {
	dv := &DrumView{}
	dv.masterVolPopup = newTestSliderPopup()
	dv.openMasterVolPopupPortal() // should not panic
}

func TestOpenMasterVolPopupPortal_NilPopup(t *testing.T) {
	dv, restore := setupPortalCloseTest(t)
	defer restore()

	dv.openMasterVolPopupPortal() // masterVolPopup is nil

	if dv.portal().Has("master-volume-popup") {
		t.Error("portal 'master-volume-popup' should NOT be open when masterVolPopup is nil")
	}
}

func TestOpenMasterVolPopupPortal_Modal(t *testing.T) {
	dv, restore := setupPortalCloseTest(t)
	defer restore()

	dv.masterVolPopup = newTestSliderPopup()
	dv.openMasterVolPopupPortal()

	if !dv.portal().HasModal() {
		t.Error("master-volume-popup portal should be modal")
	}
}

func TestOpenMasterVolPopupPortal_OnClose(t *testing.T) {
	dv, restore := setupPortalCloseTest(t)
	defer restore()

	dv.masterVolPopup = newTestSliderPopup()
	if !dv.masterVolPopup.IsOpen() {
		t.Fatal("masterVolPopup should be open before portal open")
	}

	dv.openMasterVolPopupPortal()
	dv.portal().Close("master-volume-popup")

	if dv.masterVolPopup.IsOpen() {
		t.Error("masterVolPopup should be closed after OnClose")
	}
}

// ============================================================
// openSubdivMenuPortal
// ============================================================

func TestOpenSubdivMenuPortal_NilTree(t *testing.T) {
	dv := &DrumView{}
	dv.subdivMenuComp = NewSubdivMenuComponent()
	dv.openSubdivMenuPortal() // should not panic
}

func TestOpenSubdivMenuPortal_NilComp(t *testing.T) {
	dv, restore := setupPortalCloseTest(t)
	defer restore()

	dv.openSubdivMenuPortal() // subdivMenuComp is nil

	if dv.portal().Has("subdiv-menu") {
		t.Error("portal 'subdiv-menu' should NOT be open when subdivMenuComp is nil")
	}
}

func TestOpenSubdivMenuPortal_BothNil(t *testing.T) {
	dv := &DrumView{}
	dv.openSubdivMenuPortal() // both nil, should not panic
}

func TestOpenSubdivMenuPortal_HappyPath(t *testing.T) {
	dv := newFullDrumView(t)

	// The subdiv comp must be opened before the portal wraps it, because
	// compPortalOverlay.ShouldClose() checks comp.IsOpen().
	dv.subdivMenuComp.Open()
	dv.openSubdivMenuPortal()

	if !dv.portal().Has("subdiv-menu") {
		t.Error("portal 'subdiv-menu' should be open")
	}
}

func TestOpenSubdivMenuPortal_OnClose(t *testing.T) {
	dv := newFullDrumView(t)

	dv.subdivMenuComp.Open()
	if !dv.subdivMenuComp.IsOpen() {
		t.Fatal("subdivMenuComp should be open")
	}

	dv.openSubdivMenuPortal()
	dv.portal().Close("subdiv-menu")

	if dv.subdivMenuComp.IsOpen() {
		t.Error("subdivMenuComp should be closed after portal OnClose")
	}
}

// ============================================================
// openInstMenuPortal
// ============================================================

func TestOpenInstMenuPortal_NilTree(t *testing.T) {
	dv := &DrumView{}
	dv.instMenuComp = NewInstrumentMenuComponent()
	dv.openInstMenuPortal() // should not panic
}

func TestOpenInstMenuPortal_NilComp(t *testing.T) {
	dv, restore := setupPortalCloseTest(t)
	defer restore()

	dv.openInstMenuPortal() // instMenuComp is nil

	if dv.portal().Has("inst-menu") {
		t.Error("portal 'inst-menu' should NOT be open when instMenuComp is nil")
	}
}

func TestOpenInstMenuPortal_BothNil(t *testing.T) {
	dv := &DrumView{}
	dv.openInstMenuPortal() // both nil, should not panic
}

func TestOpenInstMenuPortal_HappyPath(t *testing.T) {
	dv := newFullDrumView(t)

	dv.instMenuComp.Open()
	dv.instMenuRow = 0
	dv.openInstMenuPortal()

	if !dv.portal().Has("inst-menu") {
		t.Error("portal 'inst-menu' should be open")
	}
}

func TestOpenInstMenuPortal_NegativeRow(t *testing.T) {
	dv := newFullDrumView(t)

	dv.instMenuComp.Open()
	dv.instMenuRow = -1
	dv.openInstMenuPortal()

	// Should still open, just with zero anchor.
	if !dv.portal().Has("inst-menu") {
		t.Error("portal 'inst-menu' should be open even with negative instMenuRow")
	}
}

func TestOpenInstMenuPortal_RowOutOfBounds(t *testing.T) {
	dv := newFullDrumView(t)

	dv.instMenuComp.Open()
	dv.instMenuRow = 999 // way out of bounds
	dv.openInstMenuPortal()

	// Should still open, just with zero anchor.
	if !dv.portal().Has("inst-menu") {
		t.Error("portal 'inst-menu' should be open even with out-of-bounds instMenuRow")
	}
}

func TestOpenInstMenuPortal_OnClose(t *testing.T) {
	dv := newFullDrumView(t)

	dv.instMenuComp.Open()
	dv.instMenuRow = 0
	dv.openInstMenuPortal()

	if !dv.instMenuComp.IsOpen() {
		t.Fatal("instMenuComp should be open")
	}

	dv.portal().Close("inst-menu")

	if dv.instMenuComp.IsOpen() {
		t.Error("instMenuComp should be closed after portal OnClose")
	}
}

// ============================================================
// openColorWheelPortal
// ============================================================

func TestOpenColorWheelPortal_NilTree(t *testing.T) {
	dv := &DrumView{}
	dv.colorWheelComp = NewColorWheelComponent()
	dv.openColorWheelPortal() // should not panic
}

func TestOpenColorWheelPortal_NilComp(t *testing.T) {
	dv, restore := setupPortalCloseTest(t)
	defer restore()

	dv.openColorWheelPortal() // colorWheelComp is nil

	if dv.portal().Has("color-wheel") {
		t.Error("portal 'color-wheel' should NOT be open when colorWheelComp is nil")
	}
}

func TestOpenColorWheelPortal_BothNil(t *testing.T) {
	dv := &DrumView{}
	dv.openColorWheelPortal() // both nil, should not panic
}

func TestOpenColorWheelPortal_HappyPath(t *testing.T) {
	dv := newFullDrumView(t)

	dv.colorWheelComp.Open()
	dv.colorMenuRow = 0
	dv.openColorWheelPortal()

	if !dv.portal().Has("color-wheel") {
		t.Error("portal 'color-wheel' should be open")
	}
}

func TestOpenColorWheelPortal_NegativeRow(t *testing.T) {
	dv := newFullDrumView(t)

	dv.colorWheelComp.Open()
	dv.colorMenuRow = -1
	dv.openColorWheelPortal()

	if !dv.portal().Has("color-wheel") {
		t.Error("portal 'color-wheel' should be open even with negative colorMenuRow")
	}
}

func TestOpenColorWheelPortal_OnClose(t *testing.T) {
	dv := newFullDrumView(t)

	dv.colorWheelComp.Open()
	if !dv.colorWheelComp.IsOpen() {
		t.Fatal("colorWheelComp should be open")
	}

	dv.openColorWheelPortal()
	dv.portal().Close("color-wheel")

	if dv.colorWheelComp.IsOpen() {
		t.Error("colorWheelComp should be closed after portal OnClose")
	}
}

// ============================================================
// openRenamePortal
// ============================================================

func TestOpenRenamePortal_NilTree(t *testing.T) {
	dv := &DrumView{}
	dv.renameComp = NewRenameComponent()
	dv.openRenamePortal() // should not panic
}

func TestOpenRenamePortal_NilComp(t *testing.T) {
	dv, restore := setupPortalCloseTest(t)
	defer restore()

	dv.openRenamePortal() // renameComp is nil

	if dv.portal().Has("rename") {
		t.Error("portal 'rename' should NOT be open when renameComp is nil")
	}
}

func TestOpenRenamePortal_BothNil(t *testing.T) {
	dv := &DrumView{}
	dv.openRenamePortal() // both nil, should not panic
}

func TestOpenRenamePortal_HappyPath(t *testing.T) {
	dv := newFullDrumView(t)

	dv.renameComp.Open()
	dv.renameRow = 0
	dv.openRenamePortal()

	if !dv.portal().Has("rename") {
		t.Error("portal 'rename' should be open")
	}
}

func TestOpenRenamePortal_NegativeRow(t *testing.T) {
	dv := newFullDrumView(t)

	dv.renameComp.Open()
	dv.renameRow = -1
	dv.openRenamePortal()

	if !dv.portal().Has("rename") {
		t.Error("portal 'rename' should be open even with negative renameRow")
	}
}

// ============================================================
// Multiple portals at once
// ============================================================

func TestOpenMultiplePortals(t *testing.T) {
	dv := newFullDrumView(t)

	dv.openOverflowMenuPortal()
	dv.openContextMenuPortal()
	dv.openNamingPortal()

	if dv.portal().StackLen() != 3 {
		t.Errorf("expected 3 portal entries, got %d", dv.portal().StackLen())
	}
	if !dv.portal().Has("overflow-menu") {
		t.Error("overflow-menu should be open")
	}
	if !dv.portal().Has("context-menu") {
		t.Error("context-menu should be open")
	}
	if !dv.portal().Has("naming") {
		t.Error("naming should be open")
	}
}

func TestOpenPortal_ReplacesExisting(t *testing.T) {
	dv := newFullDrumView(t)

	dv.openFXPanelPortal()
	dv.openFXPanelPortal() // should replace, not add

	if dv.portal().StackLen() != 1 {
		t.Errorf("expected 1 entry after double-open, got %d", dv.portal().StackLen())
	}
	if !dv.portal().Has("fx-panel") {
		t.Error("fx-panel should still be open")
	}
}

func TestOpenPortal_TopID(t *testing.T) {
	dv := newFullDrumView(t)

	dv.openContextMenuPortal()
	dv.openFXPanelPortal()

	if top := dv.portal().TopID(); top != "fx-panel" {
		t.Errorf("expected TopID 'fx-panel', got %q", top)
	}
}

// ============================================================
// close* functions (0% coverage in drumview_portal_open.go)
// ============================================================

func TestCloseOverflowMenuPortal(t *testing.T) {
	dv := newFullDrumView(t)

	dv.openOverflowMenuPortal()
	if !dv.portal().Has("overflow-menu") {
		t.Fatal("portal should be open")
	}

	dv.closeOverflowMenuPortal()

	if dv.portal().Has("overflow-menu") {
		t.Error("portal 'overflow-menu' should be closed")
	}
}

func TestCloseOverflowMenuPortal_NilTree(t *testing.T) {
	dv := &DrumView{}
	dv.closeOverflowMenuPortal() // should not panic
}

func TestCloseFXPanelPortal(t *testing.T) {
	dv := newFullDrumView(t)

	dv.openFXPanelPortal()
	if !dv.portal().Has("fx-panel") {
		t.Fatal("portal should be open")
	}

	dv.closeFXPanelPortal()

	if dv.portal().Has("fx-panel") {
		t.Error("portal 'fx-panel' should be closed")
	}
}

func TestCloseFXPanelPortal_NilTree(t *testing.T) {
	dv := &DrumView{}
	dv.closeFXPanelPortal() // should not panic
}

func TestCloseVolPopupPortal(t *testing.T) {
	dv, restore := setupPortalCloseTest(t)
	defer restore()

	dv.volPopup = newTestSliderPopup()
	dv.openVolPopupPortal()
	if !dv.portal().Has("volume-popup") {
		t.Fatal("portal should be open")
	}

	dv.closeVolPopupPortal()

	if dv.portal().Has("volume-popup") {
		t.Error("portal 'volume-popup' should be closed")
	}
}

func TestCloseVolPopupPortal_NilTree(t *testing.T) {
	dv := &DrumView{}
	dv.closeVolPopupPortal() // should not panic
}

func TestCloseMasterVolPopupPortal(t *testing.T) {
	dv, restore := setupPortalCloseTest(t)
	defer restore()

	dv.masterVolPopup = newTestSliderPopup()
	dv.openMasterVolPopupPortal()
	if !dv.portal().Has("master-volume-popup") {
		t.Fatal("portal should be open")
	}

	dv.closeMasterVolPopupPortal()

	if dv.portal().Has("master-volume-popup") {
		t.Error("portal 'master-volume-popup' should be closed")
	}
}

func TestCloseMasterVolPopupPortal_NilTree(t *testing.T) {
	dv := &DrumView{}
	dv.closeMasterVolPopupPortal() // should not panic
}

func TestCloseNamingPortal(t *testing.T) {
	dv := newFullDrumView(t)

	dv.openNamingPortal()
	if !dv.portal().Has("naming") {
		t.Fatal("portal should be open")
	}

	dv.closeNamingPortal()

	if dv.portal().Has("naming") {
		t.Error("portal 'naming' should be closed")
	}
}

func TestCloseNamingPortal_NilTree(t *testing.T) {
	dv := &DrumView{}
	dv.closeNamingPortal() // should not panic
}

// ============================================================
// Overlay closure tests (exercise isOpenFn / ShouldClose / wheelFn)
// ============================================================

func TestOpenOverflowMenuPortal_ShouldClose(t *testing.T) {
	dv := newFullDrumView(t)

	dv.openOverflowMenuPortal()

	// The overlay's ShouldClose delegates to IsOverflowMenuOpen().
	// Since the portal is open, IsOverflowMenuOpen() returns true,
	// so ShouldClose() should return false.
	if dv.IsOverflowMenuOpen() != true {
		t.Error("IsOverflowMenuOpen should be true while portal is open")
	}

	// Close and verify ShouldClose would return true after portal removal.
	dv.closeOverflowMenuPortal()
	if dv.IsOverflowMenuOpen() {
		t.Error("IsOverflowMenuOpen should be false after close")
	}
}

func TestOpenContextMenuPortal_ShouldClose(t *testing.T) {
	dv := newFullDrumView(t)

	dv.openContextMenuPortal()

	if !dv.IsContextMenuOpen() {
		t.Error("IsContextMenuOpen should be true while portal is open")
	}

	dv.closeContextMenuPortal()
	if dv.IsContextMenuOpen() {
		t.Error("IsContextMenuOpen should be false after close")
	}
}

func TestOpenFXPanelPortal_ShouldClose(t *testing.T) {
	dv := newFullDrumView(t)

	dv.openFXPanelPortal()

	if !dv.IsFXPanelOpen() {
		t.Error("IsFXPanelOpen should be true while portal is open")
	}

	dv.closeFXPanelPortal()
	if dv.IsFXPanelOpen() {
		t.Error("IsFXPanelOpen should be false after close")
	}
}

func TestOpenNamingPortal_ShouldClose(t *testing.T) {
	dv := newFullDrumView(t)

	dv.openNamingPortal()

	if !dv.IsNamingOpen() {
		t.Error("IsNamingOpen should be true while portal is open")
	}

	dv.closeNamingPortal()
	if dv.IsNamingOpen() {
		t.Error("IsNamingOpen should be false after close")
	}
}

// ============================================================
// openInstMenuPortal anchor with valid row index
// ============================================================

func TestOpenInstMenuPortal_ValidRowAnchor(t *testing.T) {
	dv := newFullDrumView(t)

	dv.instMenuComp.Open()
	labels := dv.rowLabels()
	if len(labels) == 0 {
		t.Skip("no row labels available")
	}

	dv.instMenuRow = 0
	dv.openInstMenuPortal()

	if !dv.portal().Has("inst-menu") {
		t.Error("portal 'inst-menu' should be open with valid row anchor")
	}
}

// ============================================================
// openColorWheelPortal anchor with valid row index
// ============================================================

func TestOpenColorWheelPortal_ValidRowAnchor(t *testing.T) {
	dv := newFullDrumView(t)

	dv.colorWheelComp.Open()
	btns := dv.rowColorBtns()
	if len(btns) == 0 {
		t.Skip("no row color buttons available")
	}

	dv.colorMenuRow = 0
	dv.openColorWheelPortal()

	if !dv.portal().Has("color-wheel") {
		t.Error("portal 'color-wheel' should be open with valid row anchor")
	}
}

// ============================================================
// openRenamePortal anchor with valid row index
// ============================================================

func TestOpenRenamePortal_ValidRowAnchor(t *testing.T) {
	dv := newFullDrumView(t)

	dv.renameComp.Open()
	labels := dv.rowLabels()
	if len(labels) == 0 {
		t.Skip("no row labels available")
	}

	dv.renameRow = 0
	dv.openRenamePortal()

	if !dv.portal().Has("rename") {
		t.Error("portal 'rename' should be open with valid row anchor")
	}
}

// ============================================================
// Closure exercise tests — call overlay methods to cover closure bodies
// ============================================================

// portalEntryByID returns the PortalEntry with the given ID from the stack.
func portalEntryByID(p *OverlayPortal, id string) *PortalEntry {
	for i := range p.stack {
		if p.stack[i].ID == id {
			return &p.stack[i]
		}
	}
	return nil
}

func TestOpenOverflowMenuPortal_OverlayShouldClose(t *testing.T) {
	dv := newFullDrumView(t)
	dv.openOverflowMenuPortal()

	entry := portalEntryByID(dv.portal(), "overflow-menu")
	if entry == nil {
		t.Fatal("entry not found")
	}

	// ShouldClose calls isOpenFn which calls IsOverflowMenuOpen.
	if entry.Overlay.ShouldClose() {
		t.Error("ShouldClose should be false while portal is open")
	}
}

func TestOpenOverflowMenuPortal_OverlayHitAreas(t *testing.T) {
	dv := newFullDrumView(t)
	dv.openOverflowMenuPortal()

	entry := portalEntryByID(dv.portal(), "overflow-menu")
	if entry == nil {
		t.Fatal("entry not found")
	}
	// HitAreas calls rectFn (overflowPopupRect).
	_ = entry.Overlay.HitAreas()
}

func TestOpenOverflowMenuPortal_OverlayDraw(t *testing.T) {
	dv := newFullDrumView(t)
	dv.openOverflowMenuPortal()

	entry := portalEntryByID(dv.portal(), "overflow-menu")
	if entry == nil {
		t.Fatal("entry not found")
	}
	// Draw calls drawFn (drawOverflowMenu). Needs a real image.
	screen := ebiten.NewImage(800, 600)
	entry.Overlay.Draw(screen)
}

func TestOpenContextMenuPortal_OverlayShouldClose(t *testing.T) {
	dv := newFullDrumView(t)
	dv.openContextMenuPortal()

	entry := portalEntryByID(dv.portal(), "context-menu")
	if entry == nil {
		t.Fatal("entry not found")
	}
	if entry.Overlay.ShouldClose() {
		t.Error("ShouldClose should be false while portal is open")
	}
}

func TestOpenContextMenuPortal_OverlayHitAreas(t *testing.T) {
	dv := newFullDrumView(t)
	dv.openContextMenuPortal()

	entry := portalEntryByID(dv.portal(), "context-menu")
	if entry == nil {
		t.Fatal("entry not found")
	}
	_ = entry.Overlay.HitAreas()
}

func TestOpenFXPanelPortal_OverlayShouldClose(t *testing.T) {
	dv := newFullDrumView(t)
	dv.openFXPanelPortal()

	entry := portalEntryByID(dv.portal(), "fx-panel")
	if entry == nil {
		t.Fatal("entry not found")
	}
	if entry.Overlay.ShouldClose() {
		t.Error("ShouldClose should be false while portal is open")
	}
}

func TestOpenFXPanelPortal_OverlayHitAreas(t *testing.T) {
	dv := newFullDrumView(t)
	dv.openFXPanelPortal()

	entry := portalEntryByID(dv.portal(), "fx-panel")
	if entry == nil {
		t.Fatal("entry not found")
	}
	_ = entry.Overlay.HitAreas()
}

func TestOpenNamingPortal_OverlayShouldClose(t *testing.T) {
	dv := newFullDrumView(t)
	dv.openNamingPortal()

	entry := portalEntryByID(dv.portal(), "naming")
	if entry == nil {
		t.Fatal("entry not found")
	}
	// ShouldClose calls isOpenFn which calls IsNamingOpen.
	if entry.Overlay.ShouldClose() {
		t.Error("ShouldClose should be false while portal is open")
	}
}

func TestOpenNamingPortal_OverlayHitAreas(t *testing.T) {
	dv := newFullDrumView(t)
	dv.openNamingPortal()

	entry := portalEntryByID(dv.portal(), "naming")
	if entry == nil {
		t.Fatal("entry not found")
	}
	// HitAreas calls rectFn which returns dv.Bounds.
	areas := entry.Overlay.HitAreas()
	if len(areas) == 0 {
		t.Error("naming portal should have hit areas (Bounds is non-empty)")
	}
}

func TestOpenNamingPortal_OverlayUpdate_NotOpen(t *testing.T) {
	dv := newFullDrumView(t)
	dv.openNamingPortal()

	entry := portalEntryByID(dv.portal(), "naming")
	if entry == nil {
		t.Fatal("entry not found")
	}

	// Close the portal first so updateFn hits the early return.
	dv.portal().Close("naming")

	// Manually invoke Update on the overlay. Since the portal is closed,
	// IsNamingOpen returns false and the updateFn early-returns.
	if u, ok := entry.Overlay.(PortalUpdater); ok {
		u.Update()
	}
}

func TestOpenNamingPortal_OverlayUpdate_DesktopPath(t *testing.T) {
	dv := newFullDrumView(t)

	// Set pendingWAV so IsNamingOpen returns true.
	dv.pendingWAV = "test.wav"
	dv.openNamingPortal()

	entry := portalEntryByID(dv.portal(), "naming")
	if entry == nil {
		t.Fatal("entry not found")
	}

	// Call Update — exercises the desktop (non-mobile) code path which
	// creates nameBox, saveBtn, and updates text input.
	if u, ok := entry.Overlay.(PortalUpdater); ok {
		u.Update()
	}

	if dv.nameBox == nil {
		t.Error("nameBox should be initialized after Update")
	}
	if dv.saveBtn == nil {
		t.Error("saveBtn should be initialized after Update")
	}
}

func TestOpenNamingPortal_WheelFn(t *testing.T) {
	dv := newFullDrumView(t)
	dv.openNamingPortal()

	entry := portalEntryByID(dv.portal(), "naming")
	if entry == nil {
		t.Fatal("entry not found")
	}

	// Exercise the wheelFn through the overlay's HitHandler.
	overlay, ok := entry.Overlay.(*dvOverlayPortal)
	if !ok {
		t.Fatal("expected *dvOverlayPortal")
	}
	result := overlay.wheelFn(0, 0, 1)
	if result != InputConsumed {
		t.Errorf("wheelFn should return InputConsumed, got %v", result)
	}
}

func TestOpenNamingPortal_InputFn_ClickOutside(t *testing.T) {
	dv := newFullDrumView(t)
	dv.pendingWAV = "test.wav"
	dv.openNamingPortal()

	entry := portalEntryByID(dv.portal(), "naming")
	if entry == nil {
		t.Fatal("entry not found")
	}

	// First update to create nameBox.
	if u, ok := entry.Overlay.(PortalUpdater); ok {
		u.Update()
	}

	overlay, ok := entry.Overlay.(*dvOverlayPortal)
	if !ok {
		t.Fatal("expected *dvOverlayPortal")
	}

	// Click outside the nameBox rect should cancel naming.
	result := overlay.inputFn(-100, -100, true)
	if result != InputConsumed {
		t.Errorf("inputFn should return InputConsumed, got %v", result)
	}

	if dv.pendingWAV != "" {
		t.Error("pendingWAV should be cleared after click-outside")
	}
}

func TestOpenOverflowMenuPortal_WheelFn(t *testing.T) {
	dv := newFullDrumView(t)
	dv.openOverflowMenuPortal()

	entry := portalEntryByID(dv.portal(), "overflow-menu")
	if entry == nil {
		t.Fatal("entry not found")
	}

	overlay, ok := entry.Overlay.(*dvOverlayPortal)
	if !ok {
		t.Fatal("expected *dvOverlayPortal")
	}
	result := overlay.wheelFn(0, 0, 1)
	if result != InputConsumed {
		t.Errorf("wheelFn should return InputConsumed, got %v", result)
	}
}

func TestOpenContextMenuPortal_WheelFn(t *testing.T) {
	dv := newFullDrumView(t)
	dv.openContextMenuPortal()

	entry := portalEntryByID(dv.portal(), "context-menu")
	if entry == nil {
		t.Fatal("entry not found")
	}

	overlay, ok := entry.Overlay.(*dvOverlayPortal)
	if !ok {
		t.Fatal("expected *dvOverlayPortal")
	}
	result := overlay.wheelFn(0, 0, 1)
	if result != InputConsumed {
		t.Errorf("wheelFn should return InputConsumed, got %v", result)
	}
}

func TestOpenFXPanelPortal_WheelFn_NoScroll(t *testing.T) {
	dv := newFullDrumView(t)
	dv.openFXPanelPortal()

	entry := portalEntryByID(dv.portal(), "fx-panel")
	if entry == nil {
		t.Fatal("entry not found")
	}

	overlay, ok := entry.Overlay.(*dvOverlayPortal)
	if !ok {
		t.Fatal("expected *dvOverlayPortal")
	}
	// fxScrollMaxPx is 0, so the wheel handler should still return InputConsumed
	// but not modify scroll.
	result := overlay.wheelFn(0, 0, 1)
	if result != InputConsumed {
		t.Errorf("wheelFn should return InputConsumed, got %v", result)
	}
}

func TestOpenFXPanelPortal_WheelFn_WithScroll(t *testing.T) {
	dv := newFullDrumView(t)
	dv.fxScrollMaxPx = 200
	dv.fxScrollOffsetPx = 100
	dv.openFXPanelPortal()

	entry := portalEntryByID(dv.portal(), "fx-panel")
	if entry == nil {
		t.Fatal("entry not found")
	}

	overlay, ok := entry.Overlay.(*dvOverlayPortal)
	if !ok {
		t.Fatal("expected *dvOverlayPortal")
	}

	// Scroll down (negative steps = scroll down).
	overlay.wheelFn(0, 0, -3)
	if dv.fxScrollOffsetPx == 100 {
		t.Error("fxScrollOffsetPx should have changed after wheel scroll")
	}
}

func TestOpenFXPanelPortal_WheelFn_ClampMin(t *testing.T) {
	dv := newFullDrumView(t)
	dv.fxScrollMaxPx = 200
	dv.fxScrollOffsetPx = 10
	dv.openFXPanelPortal()

	entry := portalEntryByID(dv.portal(), "fx-panel")
	if entry == nil {
		t.Fatal("entry not found")
	}

	overlay := entry.Overlay.(*dvOverlayPortal)
	// Scroll up by a lot (positive steps).
	overlay.wheelFn(0, 0, 100)
	if dv.fxScrollOffsetPx < 0 {
		t.Errorf("fxScrollOffsetPx should be clamped to 0, got %d", dv.fxScrollOffsetPx)
	}
}

func TestOpenFXPanelPortal_WheelFn_ClampMax(t *testing.T) {
	dv := newFullDrumView(t)
	dv.fxScrollMaxPx = 200
	dv.fxScrollOffsetPx = 190
	dv.openFXPanelPortal()

	entry := portalEntryByID(dv.portal(), "fx-panel")
	if entry == nil {
		t.Fatal("entry not found")
	}

	overlay := entry.Overlay.(*dvOverlayPortal)
	// Scroll down by a lot (negative steps).
	overlay.wheelFn(0, 0, -100)
	if dv.fxScrollOffsetPx > 200 {
		t.Errorf("fxScrollOffsetPx should be clamped to 200, got %d", dv.fxScrollOffsetPx)
	}
}

func TestOpenOverflowMenuPortal_InputFn(t *testing.T) {
	dv := newFullDrumView(t)
	dv.openOverflowMenuPortal()

	entry := portalEntryByID(dv.portal(), "overflow-menu")
	if entry == nil {
		t.Fatal("entry not found")
	}

	overlay := entry.Overlay.(*dvOverlayPortal)
	// Call inputFn at coordinates that won't match any menu item.
	result := overlay.inputFn(-1, -1, false)
	// Should return InputIgnored since handleOverflowMenuInput returns false.
	if result != InputIgnored {
		t.Errorf("inputFn should return InputIgnored for miss coordinates, got %v", result)
	}
}

func TestOpenContextMenuPortal_InputFn(t *testing.T) {
	dv := newFullDrumView(t)
	dv.openContextMenuPortal()

	entry := portalEntryByID(dv.portal(), "context-menu")
	if entry == nil {
		t.Fatal("entry not found")
	}

	overlay := entry.Overlay.(*dvOverlayPortal)
	// Call inputFn at miss coordinates.
	result := overlay.inputFn(-1, -1, false)
	if result != InputIgnored {
		t.Errorf("inputFn should return InputIgnored for miss coordinates, got %v", result)
	}
}

func TestOpenFXPanelPortal_InputFn(t *testing.T) {
	dv := newFullDrumView(t)
	dv.openFXPanelPortal()

	entry := portalEntryByID(dv.portal(), "fx-panel")
	if entry == nil {
		t.Fatal("entry not found")
	}

	overlay := entry.Overlay.(*dvOverlayPortal)
	result := overlay.inputFn(-1, -1, false)
	if result != InputIgnored {
		t.Errorf("inputFn should return InputIgnored for miss coordinates, got %v", result)
	}
}

// ============================================================
// closeRenamePortal (not tested in the close test file)
// ============================================================

func TestCloseRenamePortal(t *testing.T) {
	dv := newFullDrumView(t)

	dv.renameComp.Open()
	dv.renameRow = 0
	dv.openRenamePortal()
	if !dv.portal().Has("rename") {
		t.Fatal("portal should be open")
	}

	dv.closeRenamePortal()

	if dv.portal().Has("rename") {
		t.Error("portal 'rename' should be closed")
	}
}

func TestCloseRenamePortal_NilTree(t *testing.T) {
	dv := &DrumView{}
	dv.closeRenamePortal() // should not panic
}

// ============================================================
// Additional closure exercise tests
// ============================================================

func TestOpenContextMenuPortal_UpdateFn(t *testing.T) {
	dv := newFullDrumView(t)
	dv.openContextMenuPortal()

	entry := portalEntryByID(dv.portal(), "context-menu")
	if entry == nil {
		t.Fatal("entry not found")
	}

	// Call Update (exercises updateFn which checks contextMenuScroll momentum).
	if u, ok := entry.Overlay.(PortalUpdater); ok {
		u.Update()
	}
}

func TestOpenContextMenuPortal_DrawFn(t *testing.T) {
	dv := newFullDrumView(t)
	dv.openContextMenuPortal()

	entry := portalEntryByID(dv.portal(), "context-menu")
	if entry == nil {
		t.Fatal("entry not found")
	}

	screen := ebiten.NewImage(800, 600)
	entry.Overlay.Draw(screen)
}

func TestOpenFXPanelPortal_DrawFn(t *testing.T) {
	dv := newFullDrumView(t)
	dv.openFXPanelPortal()

	entry := portalEntryByID(dv.portal(), "fx-panel")
	if entry == nil {
		t.Fatal("entry not found")
	}

	screen := ebiten.NewImage(800, 600)
	entry.Overlay.Draw(screen)
}

func TestOpenFXPanelPortal_UpdateFn(t *testing.T) {
	dv := newFullDrumView(t)
	dv.openFXPanelPortal()

	entry := portalEntryByID(dv.portal(), "fx-panel")
	if entry == nil {
		t.Fatal("entry not found")
	}

	// Call Update (exercises updateFn which checks fxScrollTS momentum).
	if u, ok := entry.Overlay.(PortalUpdater); ok {
		u.Update()
	}
}

func TestOpenNamingPortal_DrawFn(t *testing.T) {
	dv := newFullDrumView(t)
	dv.pendingWAV = "test.wav"
	dv.openNamingPortal()

	entry := portalEntryByID(dv.portal(), "naming")
	if entry == nil {
		t.Fatal("entry not found")
	}

	screen := ebiten.NewImage(800, 600)
	entry.Overlay.Draw(screen)

	// After drawFn, nameBox should have been created.
	if dv.nameBox == nil {
		t.Error("nameBox should be initialized by drawFn")
	}
}

func TestOpenNamingPortal_DrawFn_WithExistingNameBox(t *testing.T) {
	dv := newFullDrumView(t)
	dv.pendingWAV = "test.wav"
	dv.openNamingPortal()

	entry := portalEntryByID(dv.portal(), "naming")
	if entry == nil {
		t.Fatal("entry not found")
	}

	// Pre-create nameBox and saveBtn, then draw again to cover the
	// path where they already exist.
	screen := ebiten.NewImage(800, 600)
	entry.Overlay.Draw(screen) // first draw creates nameBox

	dv.saveBtn = NewButton("Save", UploadBtnStyle, nil)
	entry.Overlay.Draw(screen) // second draw exercises existing nameBox + saveBtn path
}

func TestOpenNamingPortal_InputFn_NoPress(t *testing.T) {
	dv := newFullDrumView(t)
	dv.pendingWAV = "test.wav"
	dv.openNamingPortal()

	entry := portalEntryByID(dv.portal(), "naming")
	if entry == nil {
		t.Fatal("entry not found")
	}

	overlay := entry.Overlay.(*dvOverlayPortal)
	// Call inputFn with pressed=false — should return InputConsumed.
	result := overlay.inputFn(0, 0, false)
	if result != InputConsumed {
		t.Errorf("inputFn should return InputConsumed for non-press, got %v", result)
	}
}

func TestOpenNamingPortal_UpdateFn_MultipleFrames(t *testing.T) {
	dv := newFullDrumView(t)
	dv.pendingWAV = "test.wav"
	dv.openNamingPortal()

	entry := portalEntryByID(dv.portal(), "naming")
	if entry == nil {
		t.Fatal("entry not found")
	}

	// Call Update twice to ensure stable behavior and namePhase advances.
	if u, ok := entry.Overlay.(PortalUpdater); ok {
		u.Update()
		u.Update()
	}

	if dv.nameBox == nil {
		t.Error("nameBox should be initialized after Update")
	}
}

func TestOpenNamingPortal_InputFn_SaveBtn(t *testing.T) {
	dv := newFullDrumView(t)
	dv.pendingWAV = "test.wav"
	dv.openNamingPortal()

	entry := portalEntryByID(dv.portal(), "naming")
	if entry == nil {
		t.Fatal("entry not found")
	}

	// Initialize saveBtn through Update.
	if u, ok := entry.Overlay.(PortalUpdater); ok {
		u.Update()
	}

	overlay := entry.Overlay.(*dvOverlayPortal)

	if dv.saveBtn != nil {
		// Click on the save button rect.
		r := dv.saveBtn.Rect()
		if !r.Empty() {
			result := overlay.inputFn(r.Min.X+1, r.Min.Y+1, true)
			if result != InputConsumed {
				t.Errorf("inputFn should return InputConsumed on save button, got %v", result)
			}
		}
	}
}

func TestOpenNamingPortal_UpdateFn_EscapeKey(t *testing.T) {
	assertDefaultParityState(t)

	// Set up input mock where Escape key returns true.
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == ebiten.KeyEscape },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)
	dv.calcLayout()

	dv.pendingWAV = "test.wav"
	dv.nameInput = "hello"
	dv.openNamingPortal()

	entry := portalEntryByID(dv.portal(), "naming")
	if entry == nil {
		t.Fatal("entry not found")
	}

	// Call Update which checks isKeyPressed(KeyEscape).
	if u, ok := entry.Overlay.(PortalUpdater); ok {
		u.Update()
	}

	if dv.pendingWAV != "" {
		t.Error("pendingWAV should be cleared after Escape key")
	}
	if dv.nameInput != "" {
		t.Error("nameInput should be cleared after Escape key")
	}
	if dv.nameBox != nil {
		t.Error("nameBox should be nil after Escape key")
	}
}

func TestOpenNamingPortal_UpdateFn_EnterKey(t *testing.T) {
	assertDefaultParityState(t)

	// Set up input mock where Enter key returns true.
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == ebiten.KeyEnter },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)
	dv.calcLayout()

	dv.pendingWAV = "test.wav"
	dv.openNamingPortal()

	entry := portalEntryByID(dv.portal(), "naming")
	if entry == nil {
		t.Fatal("entry not found")
	}

	// First Update creates the nameBox (with empty value).
	if u, ok := entry.Overlay.(PortalUpdater); ok {
		u.Update()
	}
	// Enter with empty nameBox.Value() should be a no-op (id == "").
	// This exercises the Enter key code path even if it doesn't trigger registerInstrument.
}

func TestOpenContextMenuPortal_WheelFn_WithScroll(t *testing.T) {
	dv := newFullDrumView(t)

	// Create a scroll behavior with scroll so wheelFn exercises the HasScroll branch.
	dv.contextMenuScroll = NewScrollBehavior(DropdownScrollbarStyle, 24)
	dv.contextMenuScroll.VS.Total = 20  // many items
	dv.contextMenuScroll.VS.Visible = 5 // small viewport => HasScroll = true

	dv.openContextMenuPortal()

	entry := portalEntryByID(dv.portal(), "context-menu")
	if entry == nil {
		t.Fatal("entry not found")
	}

	overlay := entry.Overlay.(*dvOverlayPortal)
	result := overlay.wheelFn(0, 0, 3)
	if result != InputConsumed {
		t.Errorf("wheelFn should return InputConsumed, got %v", result)
	}
}

func TestOpenFXPanelPortal_UpdateFn_NoMomentum(t *testing.T) {
	dv := newFullDrumView(t)
	dv.openFXPanelPortal()

	entry := portalEntryByID(dv.portal(), "fx-panel")
	if entry == nil {
		t.Fatal("entry not found")
	}

	// updateFn checks fxScrollTS.HasMomentum() which should be false
	// by default. This exercises the early-exit path.
	if u, ok := entry.Overlay.(PortalUpdater); ok {
		u.Update()
	}
}

func TestOpenOverflowMenuPortal_WheelFn_WithScroll(t *testing.T) {
	dv := newFullDrumView(t)

	// Create a scroll behavior with scroll for overflow menu.
	dv.overflowMenuScroll = NewMenuScroll(DropdownScrollbarStyle, 24)
	dv.overflowMenuScroll.Configure(image.Rect(0, 0, 100, 200), 20, 5)

	dv.openOverflowMenuPortal()

	entry := portalEntryByID(dv.portal(), "overflow-menu")
	if entry == nil {
		t.Fatal("entry not found")
	}

	overlay := entry.Overlay.(*dvOverlayPortal)
	result := overlay.wheelFn(0, 0, 3)
	if result != InputConsumed {
		t.Errorf("wheelFn should return InputConsumed, got %v", result)
	}
}

func TestOpenFXPanelPortal_OverlayShouldClose_True(t *testing.T) {
	dv := newFullDrumView(t)
	dv.openFXPanelPortal()

	entry := portalEntryByID(dv.portal(), "fx-panel")
	if entry == nil {
		t.Fatal("entry not found")
	}

	// Save the overlay reference before closing (Close zeroes the stack slot).
	overlay := entry.Overlay

	// Close the portal. IsFXPanelOpen checks portal.Has().
	dv.portal().Close("fx-panel")

	// The overlay's isOpenFn still references dv.IsFXPanelOpen() which
	// checks portal.Has("fx-panel"). Since we closed it, ShouldClose
	// should return true.
	if !overlay.ShouldClose() {
		t.Error("ShouldClose should be true after portal is closed")
	}
}
