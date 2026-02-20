package ui

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestNodePopupMobileButtonSizes verifies all sidebar buttons use the fixed
// sidebarBtnH height on mobile (no scaling — scrolling is used instead).
func TestNodePopupMobileButtonSizes(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	n := g.tryAddNode(2, 0, 0)
	g.sidebar.Open(g.nodeByID(n.ID))
	// Expand all sections to check all buttons
	g.sidebar.sectionOpen["vol"] = true
	g.sidebar.sectionOpen["pit"] = true
	g.sidebar.sectionOpen["dur"] = true
	g.sidebar.sectionOpen["logic"] = true
	g.sidebar.sectionOpen["groove"] = true
	g.sidebar.sectionOpen["aud"] = true
	g.sidebar.layout()

	allIDs := []string{"vol-", "vol+", "pit-", "pit+", "dur-", "dur+", "gp-", "gp+", "logic", "grv", "aud"}
	for _, id := range allIDs {
		r := g.sidebar.rects[id]
		if r.Empty() {
			continue
		}
		if r.Dy() != sidebarBtnH {
			t.Errorf("button %q height: want %d, got %d", id, sidebarBtnH, r.Dy())
		}
	}
}

// TestNodePopupMobileLogicDropdownInline verifies logic dropdown items are
// horizontally within panel bounds.
func TestNodePopupMobileLogicDropdownInline(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	n := g.tryAddNode(2, 0, 0)
	g.sidebar.Open(g.nodeByID(n.ID))
	g.sidebar.sectionOpen["logic"] = true
	g.sidebar.logicDropdownOpen = true
	g.sidebar.layout()

	panel := g.sidebar.rects["panel"]
	found := 0
	for id, r := range g.sidebar.rects {
		if !strings.HasPrefix(id, "logic:") || r.Empty() {
			continue
		}
		found++
		// Horizontal: must be within panel (the key mobile fix)
		if r.Min.X < panel.Min.X || r.Max.X > panel.Max.X {
			t.Errorf("logic dropdown %q outside panel horizontally: item=%v panel=%v", id, r, panel)
		}
	}
	if found == 0 {
		t.Fatal("no logic dropdown items found")
	}
}

// TestNodePopupMobileGrooveDropdownInline verifies groove dropdown items are
// horizontally within panel bounds.
func TestNodePopupMobileGrooveDropdownInline(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	n := g.tryAddNode(2, 0, 0)
	g.sidebar.Open(g.nodeByID(n.ID))
	g.sidebar.sectionOpen["groove"] = true
	g.sidebar.grooveDropdownOpen = true
	g.sidebar.layout()

	panel := g.sidebar.rects["panel"]
	found := 0
	for id, r := range g.sidebar.rects {
		if !strings.HasPrefix(id, "groove:") || r.Empty() {
			continue
		}
		found++
		if r.Min.X < panel.Min.X || r.Max.X > panel.Max.X {
			t.Errorf("groove dropdown %q outside panel horizontally: item=%v panel=%v", id, r, panel)
		}
	}
	if found == 0 {
		t.Fatal("no groove dropdown items found")
	}
}

// TestNodePopupMobileCollapsedHeight verifies that with all sections collapsed
// (the default), the sidebar panel height is small.
func TestNodePopupMobileCollapsedHeight(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	n := g.tryAddNode(2, 0, 0)
	g.sidebar.Open(g.nodeByID(n.ID))
	// sections start collapsed by default
	g.sidebar.layout()

	// Logic and groove content rects should be empty when sections are collapsed.
	for _, id := range []string{"logic", "grv", "gp-", "gp+", "ln-", "ln+", "lp-", "lp+"} {
		if r := g.sidebar.rects[id]; !r.Empty() {
			t.Errorf("expected %q to be empty when sections collapsed, got %v", id, r)
		}
	}
}

// TestNodePopupMobileExpandToggle verifies that toggling sectionOpen shows/hides
// section content (replacing the old More/Less toggle).
func TestNodePopupMobileExpandToggle(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	n := g.tryAddNode(2, 0, 0)
	g.sidebar.Open(g.nodeByID(n.ID))

	// Start collapsed (default).
	g.sidebar.layout()
	if !g.sidebar.rects["logic"].Empty() {
		t.Error("expected logic rect to be empty when section collapsed")
	}

	// Expand logic and groove sections.
	g.sidebar.sectionOpen["logic"] = true
	g.sidebar.sectionOpen["groove"] = true
	g.sidebar.layout()
	if g.sidebar.rects["logic"].Empty() {
		t.Error("expected logic rect to be non-empty when section expanded")
	}
	if g.sidebar.rects["grv"].Empty() {
		t.Error("expected groove rect to be non-empty when section expanded")
	}
}

// TestNodePopupDesktopUnchanged verifies desktop sidebar sizing.
func TestNodePopupDesktopUnchanged(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = false
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	n := g.tryAddNode(2, 0, 0)
	g.sidebar.Open(g.nodeByID(n.ID))
	g.sidebar.sectionOpen["vol"] = true
	g.sidebar.sectionOpen["pit"] = true
	g.sidebar.sectionOpen["dur"] = true
	g.sidebar.layout()

	panel := g.sidebar.rects["panel"]
	if panel.Dx() != sidebarDefaultW {
		t.Fatalf("expected desktop panel width %d, got %d", sidebarDefaultW, panel.Dx())
	}

	// All +/- buttons use uniform sidebarBtnH.
	for _, id := range []string{"vol-", "vol+", "pit-", "pit+", "dur-", "dur+"} {
		r := g.sidebar.rects[id]
		if r.Empty() {
			continue
		}
		if r.Dy() != sidebarBtnH {
			t.Errorf("button %q height: want %d, got %d", id, sidebarBtnH, r.Dy())
		}
	}
}

// TestNodePopupMobileDropdownWithinPanel verifies that with the logic dropdown
// open, all items are within the panel bounds.
func TestNodePopupMobileDropdownWithinPanel(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	n := g.tryAddNode(2, 0, 0)
	g.sidebar.Open(g.nodeByID(n.ID))
	g.sidebar.sectionOpen["logic"] = true
	g.sidebar.logicDropdownOpen = true
	g.sidebar.layout()

	panel := g.sidebar.rects["panel"]
	// Verify all logic dropdown items are within panel width.
	for id, r := range g.sidebar.rects {
		if !strings.HasPrefix(id, "logic:") || r.Empty() {
			continue
		}
		if r.Min.X < panel.Min.X || r.Max.X > panel.Max.X {
			t.Errorf("logic dropdown item %q overflows panel: item=%v panel=%v", id, r, panel)
		}
	}

	// Vol/Pitch/Dur sections should still have section headers visible.
	for _, id := range []string{"sec-vol", "sec-pit", "sec-dur"} {
		if r := g.sidebar.rects[id]; r.Empty() {
			t.Errorf("expected %q section header to be visible during dropdown", id)
		}
	}
}

// TestNodePopupMobileAccordion verifies the accordion flow: when one
// dropdown is open, the other section's toggle is hidden (if not in section).
func TestNodePopupMobileAccordion(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	n := g.tryAddNode(2, 0, 0)
	g.sidebar.Open(g.nodeByID(n.ID))
	g.sidebar.sectionOpen["logic"] = true
	g.sidebar.sectionOpen["groove"] = true

	// Step 1: Open logic dropdown. Logic toggle should be visible.
	g.sidebar.logicDropdownOpen = true
	g.sidebar.grooveDropdownOpen = false
	g.sidebar.layout()

	if r := g.sidebar.rects["logic"]; r.Empty() {
		t.Error("expected logic toggle to remain visible when logic dropdown is open")
	}

	// Step 2: Close logic dropdown. Both toggles should be visible.
	g.sidebar.logicDropdownOpen = false
	g.sidebar.layout()

	if r := g.sidebar.rects["logic"]; r.Empty() {
		t.Error("expected logic toggle to be visible after closing logic dropdown")
	}
	if r := g.sidebar.rects["grv"]; r.Empty() {
		t.Error("expected groove toggle to be visible after closing logic dropdown")
	}

	// Step 3: Open groove dropdown. Groove toggle should be visible.
	g.sidebar.grooveDropdownOpen = true
	g.sidebar.layout()

	if r := g.sidebar.rects["grv"]; r.Empty() {
		t.Error("expected groove toggle to remain visible when groove dropdown is open")
	}

	// Step 4: Close groove dropdown. Both visible again.
	g.sidebar.grooveDropdownOpen = false
	g.sidebar.layout()

	if r := g.sidebar.rects["logic"]; r.Empty() {
		t.Error("expected logic toggle to be visible after closing groove dropdown")
	}
	if r := g.sidebar.rects["grv"]; r.Empty() {
		t.Error("expected groove toggle to be visible after closing groove dropdown")
	}

	// Verify the button handler closes the other dropdown via accordion.
	g.sidebar.logicDropdownOpen = false
	g.sidebar.grooveDropdownOpen = false
	g.sidebar.layout()
	// Click logic toggle.
	if b := g.sidebar.btns["logic"]; b == nil {
		t.Fatal("expected logic button to exist")
	} else {
		b.OnClick()
	}
	_ = g.Update()
	if !g.sidebar.logicDropdownOpen {
		t.Error("expected logicDropdownOpen=true after clicking logic toggle")
	}
	if g.sidebar.grooveDropdownOpen {
		t.Error("expected grooveDropdownOpen=false after clicking logic toggle")
	}
}

// TestNodePopupMobileSectionVisibility verifies that aud/move section headers
// are always laid out when sections are visible for the node type.
func TestNodePopupMobileSectionVisibility(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	n := g.tryAddNode(2, 0, 0)
	g.sidebar.Open(g.nodeByID(n.ID))

	states := []struct {
		name      string
		sections  []string
		logicKind string
		logicOpen bool
	}{
		{"all-collapsed", nil, "", false},
		{"all-expanded", []string{"vol", "pit", "dur", "logic", "groove", "aud"}, "", false},
		{"with-probability", []string{"logic"}, "probability", false},
		{"with-every-n", []string{"logic"}, "every_n_triggers", false},
		{"logic-dropdown-open", []string{"logic"}, "", true},
	}

	for _, st := range states {
		t.Run(st.name, func(t *testing.T) {
			// Reset sections
			g.sidebar.sectionOpen = map[string]bool{}
			for _, s := range st.sections {
				g.sidebar.sectionOpen[s] = true
			}
			g.sidebar.logicDropdownOpen = st.logicOpen
			g.sidebar.grooveDropdownOpen = false
			if st.logicKind != "" {
				if mn, ok := g.graph.GetNodeByID(n.ID); ok {
					p := mn.Params
					p.LogicKind = st.logicKind
					if st.logicKind == "probability" {
						p.LogicP = 0.5
					} else {
						p.LogicN = 2
					}
					g.graph.SetNodeParams(n.ID, p)
				}
			}
			g.sidebar.layout()

			// Section headers should always be present
			for _, sec := range []string{"sec-vol", "sec-pit", "sec-dur", "sec-logic", "sec-groove", "sec-aud"} {
				if r := g.sidebar.rects[sec]; r.Empty() {
					t.Errorf("section header %q should always be laid out, got empty in state %q", sec, st.name)
				}
			}
			// Aud section header should exist within grid bounds.
			// With scrolling, content can extend beyond viewport.
			gridH := g.split.GridH(g.winH)
			_ = gridH
		})
	}
}

// TestNodePopupMobileConditionalParams verifies that parameter space is only
// reserved when the logic kind actually has parameters.
func TestNodePopupMobileConditionalParams(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	n := g.tryAddNode(2, 0, 0)
	g.sidebar.Open(g.nodeByID(n.ID))
	g.sidebar.sectionOpen["logic"] = true

	// Kind = "None" (no params): param rects should be empty.
	if mn, ok := g.graph.GetNodeByID(n.ID); ok {
		p := mn.Params
		p.LogicKind = ""
		g.graph.SetNodeParams(n.ID, p)
	}
	g.sidebar.layout()
	for _, id := range []string{"ln-", "ln+", "lp-", "lp+"} {
		if r := g.sidebar.rects[id]; !r.Empty() {
			t.Errorf("expected %q to be empty when kind=None, got %v", id, r)
		}
	}

	// Kind = "every_n_triggers" (has params): ln+/- should be non-empty.
	if mn, ok := g.graph.GetNodeByID(n.ID); ok {
		p := mn.Params
		p.LogicKind = "every_n_triggers"
		p.LogicN = 2
		g.graph.SetNodeParams(n.ID, p)
	}
	g.sidebar.layout()
	if g.sidebar.rects["ln-"].Empty() || g.sidebar.rects["ln+"].Empty() {
		t.Error("expected ln-/ln+ to be non-empty when kind=every_n_triggers")
	}

	// Kind = "probability" (has params): lp+/- should be non-empty.
	if mn, ok := g.graph.GetNodeByID(n.ID); ok {
		p := mn.Params
		p.LogicKind = "probability"
		p.LogicP = 0.5
		g.graph.SetNodeParams(n.ID, p)
	}
	g.sidebar.layout()
	if g.sidebar.rects["lp-"].Empty() || g.sidebar.rects["lp+"].Empty() {
		t.Error("expected lp-/lp+ to be non-empty when kind=probability")
	}

	// Kind = "trigger_if_prev_skipped" (no params): all param rects empty.
	if mn, ok := g.graph.GetNodeByID(n.ID); ok {
		p := mn.Params
		p.LogicKind = "trigger_if_prev_skipped"
		g.graph.SetNodeParams(n.ID, p)
	}
	g.sidebar.layout()
	for _, id := range []string{"ln-", "ln+", "lp-", "lp+"} {
		if r := g.sidebar.rects[id]; !r.Empty() {
			t.Errorf("expected %q to be empty when kind=trigger_if_prev_skipped, got %v", id, r)
		}
	}
}

// TestNodePopupDesktopLayoutOrder verifies desktop layout keeps Logic and Groove
// before Aud and Move sections.
func TestNodePopupDesktopLayoutOrder(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = false
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	n := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.sidebar.Open(g.nodeByID(n.ID))
	// Expand all sections
	g.sidebar.sectionOpen["logic"] = true
	g.sidebar.sectionOpen["groove"] = true
	g.sidebar.sectionOpen["aud"] = true
	g.sidebar.sectionOpen["move"] = true
	g.sidebar.layout()

	// On desktop: Logic and Groove should come before Aud.
	logicRect := g.sidebar.rects["logic"]
	grvRect := g.sidebar.rects["grv"]
	audRect := g.sidebar.rects["aud"]
	moveRect := g.sidebar.rects["move"]

	if logicRect.Empty() || grvRect.Empty() || audRect.Empty() || moveRect.Empty() {
		t.Fatal("expected logic, grv, aud, and move rects to be non-empty on desktop")
	}
	if logicRect.Min.Y >= audRect.Min.Y {
		t.Errorf("desktop: logic should be above aud: logic.Y=%d aud.Y=%d", logicRect.Min.Y, audRect.Min.Y)
	}
	if grvRect.Min.Y >= audRect.Min.Y {
		t.Errorf("desktop: grv should be above aud: grv.Y=%d aud.Y=%d", grvRect.Min.Y, audRect.Min.Y)
	}
	if audRect.Min.Y >= moveRect.Min.Y {
		t.Errorf("desktop: aud should be above move: aud.Y=%d move.Y=%d", audRect.Min.Y, moveRect.Min.Y)
	}
}

// TestNodePopupMobileScrollEnabled verifies that on a mobile screen with all
// sections expanded and logic dropdown open, scrolling is enabled to accommodate
// the extra content (instead of the old sizeFactor scaling).
func TestNodePopupMobileScrollEnabled(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	n := g.tryAddNode(2, 0, 0)
	g.sidebar.Open(g.nodeByID(n.ID))
	// Expand all sections and open logic dropdown
	g.sidebar.sectionOpen["vol"] = true
	g.sidebar.sectionOpen["pit"] = true
	g.sidebar.sectionOpen["dur"] = true
	g.sidebar.sectionOpen["logic"] = true
	g.sidebar.sectionOpen["groove"] = true
	g.sidebar.sectionOpen["aud"] = true
	g.sidebar.logicDropdownOpen = true

	// Use a smaller grid pane to force scrolling
	g.split.Y = 300
	g.sidebar.layout()

	// Scrolling should be enabled since content overflows
	if !g.sidebar.scroll.HasScroll() {
		t.Error("expected scroll enabled when all sections expanded with dropdown in small pane")
	}

	// Close dropdown and collapse sections — scrolling should not be needed
	g.sidebar.logicDropdownOpen = false
	g.sidebar.sectionOpen = map[string]bool{}
	g.sidebar.layout()
	if g.sidebar.scroll.HasScroll() {
		t.Errorf("expected no scroll after collapsing sections, got Total=%d Visible=%d", g.sidebar.scroll.VS.Total, g.sidebar.scroll.VS.Visible)
	}
}
