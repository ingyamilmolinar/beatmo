//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestEQPeekTap_FullyEntersAudioMode verifies the EQ peek tap path
// performs the SAME side effects as the toolbar view-switch button.
// Today eqPeekHitAdapter.OnPress mirrors only the load-bearing fields
// (drumview_layout.go:923-928), missing popup-close + scroll-reset.
// After Phase 2 both paths share setViewMode().
//
// We call OnPress directly (bypassing the tree's portal-close gate)
// to test the adapter's own side-effect contract: it should call
// setViewMode, which calls CloseAllPopups — leaving the context menu
// closed when audio mode is entered regardless of how the adapter is
// reached.
func TestEQPeekTap_FullyEntersAudioMode(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)
	advanceFrames(g, 2)

	dv := g.drum

	// Open a popup to verify the adapter closes it.
	dv.OpenContextMenu(0)
	if !dv.IsContextMenuOpen() {
		t.Fatalf("precondition: context menu should be open")
	}

	r := dv.eqPeekRect
	if r.Empty() {
		t.Fatalf("precondition: eqPeekRect non-empty on collapsed mobile")
	}
	mid := r.Min.X + r.Dx()/2
	midy := r.Min.Y + r.Dy()/2

	// Call OnPress directly on the adapter — this is the path that needs to
	// close popups. (The tree's portal-close gate would close the menu before
	// routing to the adapter; we test the adapter itself here to verify the
	// post-refactor contract holds even for callers that reach the adapter
	// through non-tree paths, e.g., future setViewMode direct calls.)
	h := &eqPeekHitAdapter{dv: dv}
	result := h.OnPress(mid, midy)
	if result == InputIgnored {
		t.Fatalf("eqPeekHitAdapter.OnPress returned InputIgnored unexpectedly")
	}

	if dv.mobileEQCollapsed {
		t.Errorf("expected mobileEQCollapsed=false after peek tap")
	}
	if dv.currentViewMode != viewModeAudio {
		t.Errorf("expected currentViewMode=viewModeAudio, got %v", dv.currentViewMode)
	}
	if dv.IsContextMenuOpen() {
		t.Errorf("expected context menu CLOSED after peek tap (parity with toolbar button); still open")
	}
}
