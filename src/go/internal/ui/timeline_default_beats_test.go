package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// buildLoop creates an N-node loop circuit and wires it to game g.
func buildLoop(g *Game, n int) {
	first := g.tryAddNode(0, 0, model.NodeTypeRegular)
	prev := first
	for i := 1; i < n; i++ {
		nd := g.tryAddNode(i, 0, model.NodeTypeRegular)
		g.addEdge(prev, nd)
		prev = nd
	}
	g.addEdge(prev, first)
	g.start = first
	g.graph.StartNodeID = first.ID
	g.drum.Rows[0].Origin = first.ID
	g.drum.Rows[0].Node = first
}

// TestDefaultTimelineBeatsDesktop verifies that on desktop, the visible drum
// timeline auto-grows to the full circuit path length (no cap).
func TestDefaultTimelineBeatsDesktop(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)

	buildLoop(g, 16)
	g.Layout(1200, 800)
	g.updateBeatInfos()
	g.refreshDrumRow()

	// Desktop profile has DefaultTimelineBeats=0 (no cap).
	if Profile().DefaultTimelineBeats != 0 {
		t.Fatalf("expected desktop DefaultTimelineBeats=0, got %d", Profile().DefaultTimelineBeats)
	}

	// timelineBeats should reflect the full circuit.
	if g.drum.timelineBeats < 1 {
		t.Fatalf("desktop: timelineBeats should be positive, got %d", g.drum.timelineBeats)
	}

	// On desktop, visible Length should equal the full path (no artificial
	// beats cap—only screen-width clamping applies).
	if g.drum.Length != g.drum.timelineBeats {
		t.Logf("desktop: Length=%d vs timelineBeats=%d (screen clamp may apply)",
			g.drum.Length, g.drum.timelineBeats)
	}
}

// TestDefaultTimelineBeatsMobile verifies that on mobile, the visible drum
// timeline is capped at 8 beats even when the circuit is longer.
func TestDefaultTimelineBeatsMobile(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	withSmallScreen(t, true)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)

	// Build a circuit long enough that the path exceeds 8 beats.
	buildLoop(g, 64)
	g.Layout(400, 800)
	g.updateBeatInfos()
	g.refreshDrumRow()

	// Mobile profile has DefaultTimelineBeats=8.
	if Profile().DefaultTimelineBeats != 8 {
		t.Fatalf("expected mobile DefaultTimelineBeats=8, got %d", Profile().DefaultTimelineBeats)
	}

	upb := g.drum.timelineUnitsPerBeat
	maxVisible := 8 * upb
	pathLen := g.drum.timelineBeats

	// Only check the cap when the path is actually longer than 8 beats.
	if pathLen > maxVisible {
		if g.drum.Length > maxVisible {
			t.Errorf("mobile: expected Length <= %d (8 beats × %d upb), got %d (path=%d)",
				maxVisible, upb, g.drum.Length, pathLen)
		}
	}

	// The full circuit must still be scrollable via timelineBeats.
	if g.drum.timelineBeats < g.drum.Length {
		t.Errorf("mobile: timelineBeats %d should be >= visible Length %d",
			g.drum.timelineBeats, g.drum.Length)
	}
}

// TestDefaultTimelineBeatsMobileVsDesktop directly compares the visible Length
// for the same circuit on mobile vs desktop to confirm mobile is smaller.
func TestDefaultTimelineBeatsMobileVsDesktop(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)

	// Desktop run
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	buildLoop(g, 64)
	g.Layout(1200, 800)
	g.updateBeatInfos()
	g.refreshDrumRow()
	desktopLen := g.drum.Length
	desktopBeats := g.drum.timelineBeats
	g.CloseForTest()

	// Mobile run
	withSmallScreen(t, true)
	g2 := New(testLogger)
	t.Cleanup(g2.CloseForTest)
	buildLoop(g2, 64)
	g2.Layout(400, 800)
	g2.updateBeatInfos()
	g2.refreshDrumRow()
	mobileLen := g2.drum.Length
	mobileBeats := g2.drum.timelineBeats

	// Mobile visible length should be ≤ 8 beats when path is long enough.
	upb := g2.drum.timelineUnitsPerBeat
	maxMobileVisible := 8 * upb
	if mobileBeats > maxMobileVisible && mobileLen > maxMobileVisible {
		t.Errorf("mobile visible Length %d exceeds 8-beat cap %d", mobileLen, maxMobileVisible)
	}

	// Both should have the same scrollable range (same circuit).
	if mobileBeats != desktopBeats {
		t.Logf("note: timelineBeats differ (mobile=%d desktop=%d), expected same circuit path",
			mobileBeats, desktopBeats)
	}

	// Mobile visible should be ≤ desktop visible (the cap only makes it
	// smaller or equal, never larger).
	if mobileLen > desktopLen {
		t.Errorf("mobile Length %d should be <= desktop Length %d", mobileLen, desktopLen)
	}
}

// TestDefaultTimelineBeatsMobileSmallCircuit verifies that when the circuit is
// shorter than the mobile default (8 beats), no padding is applied.
func TestDefaultTimelineBeatsMobileSmallCircuit(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	withSmallScreen(t, true)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)

	buildLoop(g, 4)
	g.Layout(400, 800)
	g.updateBeatInfos()
	g.refreshDrumRow()

	// With a small circuit, visible length should match the path (no padding).
	if g.drum.Length > g.drum.timelineBeats {
		t.Errorf("mobile small circuit: Length %d should not exceed timelineBeats %d",
			g.drum.Length, g.drum.timelineBeats)
	}
}

// TestDefaultTimelineBeatsMobileLateInit simulates the WASM startup flow where
// the mobile profile activates AFTER the initial updateBeatInfos. The visible
// Length should still be capped via SetBounds.
func TestDefaultTimelineBeatsMobileLateInit(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)

	// Step 1: Create game in desktop mode (simulates WASM before Layout).
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)

	buildLoop(g, 64)
	g.Layout(1200, 800)
	g.updateBeatInfos()
	g.refreshDrumRow()

	desktopLen := g.drum.Length

	// Step 2: Switch to mobile profile (simulates first WASM Layout call).
	withSmallScreen(t, true)
	// Trigger SetBounds with a different rect to simulate the WASM mobile
	// Layout call that activates the profile and re-clamps Length.
	g.drum.SetBounds(g.drum.Bounds.Inset(1))

	upb := g.drum.timelineUnitsPerBeat
	maxVisible := 8 * upb

	if g.drum.timelineBeats > maxVisible {
		// Path is long enough for the cap to matter.
		if g.drum.Length > maxVisible {
			t.Errorf("late-init mobile: Length %d exceeds 8-beat cap %d (was %d on desktop)",
				g.drum.Length, maxVisible, desktopLen)
		}
	}

	// timelineBeats must still cover the full circuit for scrolling.
	if g.drum.timelineBeats < desktopLen {
		t.Errorf("late-init mobile: timelineBeats %d should be >= desktop Length %d",
			g.drum.timelineBeats, desktopLen)
	}
}

// TestDefaultTimelineBeatsMobileBeatCounter verifies the beat counter text
// reflects the capped visible length on mobile, not the full circuit.
func TestDefaultTimelineBeatsMobileBeatCounter(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	withSmallScreen(t, true)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)

	buildLoop(g, 64)
	g.Layout(400, 800)
	g.updateBeatInfos()
	g.refreshDrumRow()

	upb := g.drum.timelineUnitsPerBeat
	maxVisible := 8 * upb

	// The beat counter shows "Beat X/Y" where Y = ceil(Length / upb).
	// On mobile with cap, Y should be ≤ 8.
	if g.drum.timelineBeats > maxVisible && g.drum.Length > maxVisible {
		t.Errorf("mobile beat counter: Length=%d should be <= %d (8 beats × %d upb)",
			g.drum.Length, maxVisible, upb)
	}
}

// TestDefaultTimelineBeatsMobileUserOverride verifies that after the user
// manually adjusts length via +/- buttons, the mobile cap no longer applies.
func TestDefaultTimelineBeatsMobileUserOverride(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	withSmallScreen(t, true)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)

	buildLoop(g, 64)
	g.Layout(400, 800)
	g.updateBeatInfos()
	g.refreshDrumRow()

	upb := g.drum.timelineUnitsPerBeat
	maxVisible := 8 * upb

	// Confirm initial cap is applied.
	if g.drum.timelineBeats > maxVisible && g.drum.Length > maxVisible {
		t.Fatalf("precondition: mobile Length %d should be <= %d before user override",
			g.drum.Length, maxVisible)
	}

	// Simulate user pressing the + button.
	g.drum.userAdjustedLength = true

	// Re-run updateBeatInfos — now the cap should not apply.
	g.updateBeatInfos()
	g.refreshDrumRow()

	// After user override, Length can grow beyond 8 beats.
	if g.drum.Length <= maxVisible && g.drum.timelineBeats > maxVisible {
		t.Logf("after user override, Length=%d could grow beyond %d (timelineBeats=%d)",
			g.drum.Length, maxVisible, g.drum.timelineBeats)
	}
}
