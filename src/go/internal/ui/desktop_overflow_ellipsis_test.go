//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// newDesktopGameForTransport builds a desktop-profile Game laid out at a
// realistic 1280x720 viewport. The transport zone then occupies the narrow
// left column (the timeline takes the right side), which is the layout that
// triggered the bug.
func newDesktopGameForTransport(t *testing.T) *Game {
	t.Helper()
	assertDefaultParityState(t)
	withSmallScreen(t, false) // desktop profile
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.updateBeatInfos()
	g.Layout(1280, 720)
	return g
}

// TestDesktopTransportShowsSingleOverflowEllipsis verifies the desktop
// transport exposes exactly ONE working overflow ("...") button — matching the
// mobile behaviour — rather than the two cramped Undo/Redo buttons that
// collapsed to "..." via clipTextToWidth and opened no menu.
//
// Bug: on desktop the transport zone is only ~308px wide, so the text-labelled
// Undo/Redo buttons (~21px) rendered as "..." while the real overflow button
// was hidden (rect cleared in layoutDesktop). The user saw "two ellipsis"
// buttons, neither of which opened the overflow menu.
func TestDesktopTransportShowsSingleOverflowEllipsis(t *testing.T) {
	g := newDesktopGameForTransport(t)
	dv := g.drum

	// The genuine overflow "..." button must be visible on desktop.
	if got := dv.overflowBtn().Rect(); got.Empty() {
		t.Errorf("desktop overflow button rect is empty — the '...' overflow menu trigger is hidden on desktop (want visible, like mobile)")
	}

	// File-ops move behind the overflow menu (they already exist as overflow
	// entries), so the cramped inline Upload/Import/Export buttons are hidden.
	if got := dv.uploadBtn().Rect(); !got.Empty() {
		t.Errorf("upload button rect = %v, want empty (moved behind overflow menu)", got)
	}
	if got := dv.importBtn().Rect(); !got.Empty() {
		t.Errorf("import button rect = %v, want empty (moved behind overflow menu)", got)
	}
	if got := dv.exportBtn().Rect(); !got.Empty() {
		t.Errorf("export button rect = %v, want empty (moved behind overflow menu)", got)
	}
}

// TestDesktopUndoRedoUseIconsNotEllipsisText verifies Undo/Redo stay inline on
// desktop but render as ICONS, so the narrow cells no longer collapse their
// "Undo"/"Redo" text to "..." (the source of the phantom second/third ellipsis).
func TestDesktopUndoRedoUseIconsNotEllipsisText(t *testing.T) {
	g := newDesktopGameForTransport(t)
	tz := g.drum.transportZone

	if tz.undoBtn == nil || tz.redoBtn == nil {
		t.Fatal("undo/redo buttons missing on desktop")
	}
	for name, b := range map[string]*Button{"undo": tz.undoBtn, "redo": tz.redoBtn} {
		if b.Rect().Empty() {
			t.Errorf("%s button rect is empty — should stay inline on desktop", name)
		}
		if b.Icon == "" {
			t.Errorf("%s button has no Icon — text label collapses to \"...\" in the narrow cell", name)
		}
		if b.Text != "" {
			t.Errorf("%s button still has Text=%q — icon buttons must not carry clippable text", name, b.Text)
		}
		// Whatever width the cell ends up, the rendered label must never be the
		// bare "..." ellipsis that the user mistook for an overflow button.
		clipped := clipTextToWidth(b.Text, b.Rect().Dx())
		if clipped == "..." {
			t.Errorf("%s button still renders as \"...\" (rect width=%d)", name, b.Rect().Dx())
		}
	}
}

// TestDesktopOverflowButtonOpensMenu verifies that clicking the desktop
// overflow "..." button actually opens the overflow menu — "no user action
// triggers the menu" was the reported breakage.
func TestDesktopOverflowButtonOpensMenu(t *testing.T) {
	g := newDesktopGameForTransport(t)
	dv := g.drum

	r := dv.overflowBtn().Rect()
	if r.Empty() {
		t.Fatal("overflow button rect empty — cannot click it")
	}
	cx, cy := r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2

	const W, H = 1280, 720
	reset := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	reset()

	if !dv.IsOverflowMenuOpen() {
		t.Fatalf("clicking desktop overflow button did not open the overflow menu")
	}

	// The popup must land somewhere visible (non-empty, inside the drum pane),
	// otherwise the menu "opens" but renders off-screen.
	popup := dv.overflowPopupRect()
	if popup.Empty() {
		t.Errorf("overflow popup rect is empty after opening on desktop")
	}
	if !popup.In(dv.Bounds) {
		t.Errorf("overflow popup rect %v not within drum bounds %v", popup, dv.Bounds)
	}
}
