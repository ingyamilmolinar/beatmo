package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// secHeaderCenter opens the sidebar on node id and returns the center of the
// "grp" section header, with the section collapsed (Open() resets sections).
func secHeaderCenter(t *testing.T, g *Game, id model.NodeID) (int, int) {
	t.Helper()
	g.sidebar.Open(g.nodesByID[id])
	g.sidebar.layout()
	r, ok := g.sidebar.rects["sec-grp"]
	if !ok || r.Empty() {
		t.Fatalf("setup: sec-grp rect missing (rects=%v)", g.sidebar.rects)
	}
	if g.sidebar.sectionOpen["grp"] {
		t.Fatalf("setup: grp must start collapsed")
	}
	return (r.Min.X + r.Max.X) / 2, (r.Min.Y + r.Max.Y) / 2
}

// holdClick drives a real press held for holdFrames, then a release, through
// Game.Update. When suppress is set, suppressClicksUntilRelease is raised
// AFTER SetInputForTest (which clears it) so it is live for the press —
// exactly what Button.ConsumeOnPress does at uigrid.go's press edge.
func holdClick(g *Game, x, y, holdFrames int, suppress bool) {
	down := true
	restore := SetInputForTest(
		func() (int, int) { return x, y },
		func(b ebiten.MouseButton) bool { return down && b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	defer restore()
	if suppress {
		SuppressClicksUntilMouseUp()
	}
	advanceFrames(g, holdFrames)
	down = false
	advanceFrames(g, 2)
}

// TestSidebarSectionHeaderPressWhileSuppressedDoesNotToggle pins the
// suppression contract for the sidebar's section headers.
//
// suppressClicksUntilRelease is raised by Button.ConsumeOnPress (uigrid.go)
// so that the SAME held press cannot go on to activate anything else until
// the user lets go. DeferredTap.Begin documents this as "uniform guard
// behavior across all menus", and every other sidebar control obeys it.
//
// The section-header branch did not: it toggled on raw `pressed`, so a press
// that began while clicks were suppressed still expanded the section.
//
// Driven through HandleInput directly (mirrors
// TestSidebarSectionHeaderHeldPressNeverRepeats below) rather than holdClick's
// full Game.Update() pass. The sidebar now ALSO renders/hit-tests through the
// global OverlayPortal (game_node_sidebar.go), so a full Update() dispatches
// this SAME press twice in one frame: once via g.inputDispatcher (which
// honors suppressClicksUntilRelease as set), and again via the DrumView's
// RootTree, whose tree-managed dispatch temporarily clears the GLOBAL
// suppress flag around its own hit.Handler.OnPress call (drumview_tree.go) —
// by design, since native tree zones track their own consumption via the
// tree's t.suppress instead. That second, tree-driven call lets
// DeferredTap.Begin succeed and capture the gesture, which then fires the
// tap unconditionally on release (fireTapAt has no suppress check of its
// own) — an interaction between the tree's dispatch and the sidebar's
// cross-cutting global guard that is orthogonal to the invariant this test
// pins. Calling HandleInput once, directly, isolates the section-header
// branch's own suppress check under test from that dispatch interaction.
func TestSidebarSectionHeaderPressWhileSuppressedDoesNotToggle(t *testing.T) {
	g, aID, _ := buildGroupSidebarFixture(t)
	cx, cy := secHeaderCenter(t, g, aID)

	suppressClicksUntilRelease = true
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	g.sidebar.HandleInput(cx, cy, true)
	g.sidebar.HandleInput(cx, cy, false)

	if g.sidebar.sectionOpen["grp"] {
		t.Fatalf("a press that began while clicks were suppressed must not toggle the section")
	}
}

// TestSidebarSectionHeaderHeldPressNeverRepeats pins the press-EDGE contract.
//
// The toggle fired on raw `pressed`, with no edge detection — so every frame
// the button stayed down flipped the section again. It only escaped notice
// because the normal path never reaches it (a press inside the scroll
// viewport is captured by the deferred-tap path first); the branch is
// reachable when DeferredTap.Begin declines, i.e. exactly when clicks are
// suppressed. Held for N frames it produced N toggles, leaving the section
// open or closed purely on the parity of how long the user held.
//
// Driven through HandleInput rather than Game.Update because Game.Update's
// drum.Update() rewrites the global from the DrumViewTree every frame
// (drumview_tree.go: "Tree is the single source of truth for suppress"),
// which would clear the flag mid-hold and hide the repeat.
func TestSidebarSectionHeaderHeldPressNeverRepeats(t *testing.T) {
	g, aID, _ := buildGroupSidebarFixture(t)
	cx, cy := secHeaderCenter(t, g, aID)

	suppressClicksUntilRelease = true
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	toggles := 0
	prev := g.sidebar.sectionOpen["grp"]
	for i := 0; i < 6; i++ {
		g.sidebar.HandleInput(cx, cy, true)
		if g.sidebar.sectionOpen["grp"] != prev {
			toggles++
			prev = g.sidebar.sectionOpen["grp"]
		}
	}
	g.sidebar.HandleInput(cx, cy, false)
	if g.sidebar.sectionOpen["grp"] != prev {
		toggles++
	}

	if toggles != 0 {
		t.Fatalf("a suppressed press held for 6 frames must not toggle the section, got %d toggles (per-frame repeat)", toggles)
	}
}

// TestSidebarSectionHeaderClickTogglesExactlyOnce is the regression guard for
// the normal (unsuppressed) path: one click = one toggle, whatever the hold
// duration. This is what the deferred-tap path already delivers; it must keep
// holding once the header branch is edge-gated.
//
// The card is a compact, node-anchored panel that resizes (and can flip
// above/below its node) with its content, so expanding "grp" via the first
// click can move the header to a different on-screen position for the
// second click. Re-derive the header's actual current center from
// g.sidebar.rects before each click — exactly what a real user does by
// looking at the screen — instead of reusing a coordinate captured before
// the panel moved.
func TestSidebarSectionHeaderClickTogglesExactlyOnce(t *testing.T) {
	for _, hold := range []int{1, 2, 5, 10} {
		g, aID, _ := buildGroupSidebarFixture(t)
		cx, cy := secHeaderCenter(t, g, aID)

		holdClick(g, cx, cy, hold, false)
		if !g.sidebar.sectionOpen["grp"] {
			t.Fatalf("hold=%d: one click must expand the section (got collapsed)", hold)
		}

		g.sidebar.layout()
		r, ok := g.sidebar.rects["sec-grp"]
		if !ok || r.Empty() {
			t.Fatalf("hold=%d: sec-grp rect missing after expand (rects=%v)", hold, g.sidebar.rects)
		}
		cx2, cy2 := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2

		holdClick(g, cx2, cy2, hold, false)
		if g.sidebar.sectionOpen["grp"] {
			t.Fatalf("hold=%d: a second click must collapse it again (got expanded)", hold)
		}
	}
}

// TestSidebarSectionHeaderEdgeClearsAfterCapturedGesture guards the
// press-edge bookkeeping itself.
//
// The edge flag is updated at the TOP of HandleInput, not inside the
// section-header branch, because most releases never reach that branch — the
// scroll / deferred-tap steps consume them and return early. Tracked at the
// branch, the flag stayed stuck "held" after any captured gesture, so a later
// press that DID reach the branch was misread as a continuation of the old
// one and silently swallowed.
//
// The sequence below is the one that strands the flag: a suppressed press
// (the only way to reach the branch) followed by the deferred-tap path
// capturing the rest of the gesture, so neither the held frames nor the
// release ever reach the branch again.
func TestSidebarSectionHeaderEdgeClearsAfterCapturedGesture(t *testing.T) {
	g, aID, _ := buildGroupSidebarFixture(t)
	cx, cy := secHeaderCenter(t, g, aID)

	// Frame 1: suppressed press — reaches the header branch, toggles nothing.
	suppressClicksUntilRelease = true
	t.Cleanup(func() { suppressClicksUntilRelease = false })
	g.sidebar.HandleInput(cx, cy, true)

	// Frame 2: the tree clears the flag mid-hold (drumview_tree.go owns it),
	// so the still-held press is picked up by the deferred-tap path, which
	// captures — the header branch is not reached again.
	suppressClicksUntilRelease = false
	g.sidebar.HandleInput(cx, cy, true)

	// Release: consumed by the touch/deferred-tap step, also short of the branch.
	g.sidebar.HandleInput(cx, cy, false)

	if g.sidebar.secHeldPrev {
		t.Fatalf("pointer is released but the sidebar still records a held press; the next press edge on a section header would be swallowed")
	}
}
