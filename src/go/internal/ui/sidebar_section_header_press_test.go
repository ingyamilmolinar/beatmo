package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Collapsed: Open() resets every section.
func secHeaderCenter(t *testing.T, g *Game, id model.NodeID) (int, int) {
	t.Helper()
	g.sidebar.Open(g.nodesByID[id])
	g.sidebar.layout()
	r, ok := g.sidebar.rects[sbSectionHeader(secGrp)]
	if !ok || r.Empty() {
		t.Fatalf("setup: sec-grp rect missing (rects=%v)", g.sidebar.rects)
	}
	if g.sidebar.sectionOpen[secGrp] {
		t.Fatalf("setup: grp must start collapsed")
	}
	return (r.Min.X + r.Max.X) / 2, (r.Min.Y + r.Max.Y) / 2
}

// Suppression is raised after SetInputForTest (which clears it), as Button.ConsumeOnPress does on its press edge.
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

// A press that began while clicks are suppressed must not toggle a header, as every other control obeys.
// HandleInput directly: a full Update dispatches the press twice, and the tree clears the global flag around its
// own call, an interaction outside this invariant.
func TestSidebarSectionHeaderPressWhileSuppressedDoesNotToggle(t *testing.T) {
	g, aID, _ := buildGroupSidebarFixture(t)
	cx, cy := secHeaderCenter(t, g, aID)

	suppressClicksUntilRelease = true
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	g.sidebar.HandleInput(cx, cy, true)
	g.sidebar.HandleInput(cx, cy, false)

	if g.sidebar.sectionOpen[secGrp] {
		t.Fatalf("a press that began while clicks were suppressed must not toggle the section")
	}
}

// One toggle per press edge, never one per held frame (the branch is reached when DeferredTap.Begin declines).
// HandleInput directly: drum.Update rewrites the flag from the tree each frame and would hide the repeat.
func TestSidebarSectionHeaderHeldPressNeverRepeats(t *testing.T) {
	g, aID, _ := buildGroupSidebarFixture(t)
	cx, cy := secHeaderCenter(t, g, aID)

	suppressClicksUntilRelease = true
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	toggles := 0
	prev := g.sidebar.sectionOpen[secGrp]
	for i := 0; i < 6; i++ {
		g.sidebar.HandleInput(cx, cy, true)
		if g.sidebar.sectionOpen[secGrp] != prev {
			toggles++
			prev = g.sidebar.sectionOpen[secGrp]
		}
	}
	g.sidebar.HandleInput(cx, cy, false)
	if g.sidebar.sectionOpen[secGrp] != prev {
		toggles++
	}

	if toggles != 0 {
		t.Fatalf("a suppressed press held for 6 frames must not toggle the section, got %d toggles (per-frame repeat)", toggles)
	}
}

// One click, one toggle. The card resizes and may flip, so each click re-reads the header's current centre, as
// a user does.
func TestSidebarSectionHeaderClickTogglesExactlyOnce(t *testing.T) {
	for _, hold := range []int{1, 2, 5, 10} {
		g, aID, _ := buildGroupSidebarFixture(t)
		cx, cy := secHeaderCenter(t, g, aID)

		holdClick(g, cx, cy, hold, false)
		if !g.sidebar.sectionOpen[secGrp] {
			t.Fatalf("hold=%d: one click must expand the section (got collapsed)", hold)
		}

		g.sidebar.layout()
		r, ok := g.sidebar.rects[sbSectionHeader(secGrp)]
		if !ok || r.Empty() {
			t.Fatalf("hold=%d: sec-grp rect missing after expand (rects=%v)", hold, g.sidebar.rects)
		}
		cx2, cy2 := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2

		holdClick(g, cx2, cy2, hold, false)
		if g.sidebar.sectionOpen[secGrp] {
			t.Fatalf("hold=%d: a second click must collapse it again (got expanded)", hold)
		}
	}
}

// The edge flag updates at the top of HandleInput, since most releases never reach the header branch; tracked
// there, it stuck after a captured gesture and swallowed the next real press.
func TestSidebarSectionHeaderEdgeClearsAfterCapturedGesture(t *testing.T) {
	g, aID, _ := buildGroupSidebarFixture(t)
	cx, cy := secHeaderCenter(t, g, aID)

	// Frame 1: suppressed press — reaches the header branch, toggles nothing.
	suppressClicksUntilRelease = true
	t.Cleanup(func() { suppressClicksUntilRelease = false })
	g.sidebar.HandleInput(cx, cy, true)

	// The tree clears the flag mid-hold, so the deferred-tap path captures the rest.
	suppressClicksUntilRelease = false
	g.sidebar.HandleInput(cx, cy, true)

	// Release: consumed by the touch/deferred-tap step, also short of the branch.
	g.sidebar.HandleInput(cx, cy, false)

	if g.sidebar.secHeldPrev {
		t.Fatalf("pointer is released but the sidebar still records a held press; the next press edge on a section header would be swallowed")
	}
}
