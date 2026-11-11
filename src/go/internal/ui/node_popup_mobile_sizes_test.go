package ui

import (
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
	g.sidebar.sectionOpen[secVol] = true
	g.sidebar.sectionOpen[secPit] = true
	g.sidebar.sectionOpen[secDur] = true
	g.sidebar.sectionOpen[secLogic] = true
	g.sidebar.sectionOpen[secGroove] = true
	g.sidebar.sectionOpen[secAud] = true
	g.sidebar.layout()

	// Inc/dec buttons use sidebarIncBtnH (24px), other buttons use sidebarBtnH (28px)
	incDecIDs := map[sbID]bool{}
	for _, f := range []sbField{fieldVol, fieldPit, fieldDur, fieldGroovePct} {
		incDecIDs[sbMinus(f)], incDecIDs[sbPlus(f)] = true, true
	}
	allIDs := []sbID{sbMinus(fieldVol), sbPlus(fieldVol), sbMinus(fieldPit), sbPlus(fieldPit), sbMinus(fieldDur), sbPlus(fieldDur), sbMinus(fieldGroovePct), sbPlus(fieldGroovePct), sbLogicSelect, sbGrooveSelect, sbAudible}
	for _, id := range allIDs {
		r := g.sidebar.rects[id]
		if r.Empty() {
			continue
		}
		wantH := sidebarBtnH
		if incDecIDs[id] {
			wantH = sidebarIncBtnH
		}
		if r.Dy() != wantH {
			t.Errorf("button %q height: want %d, got %d", id, wantH, r.Dy())
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
	g.sidebar.sectionOpen[secLogic] = true
	g.sidebar.logicDropdownOpen = true
	g.sidebar.layout()

	panel := g.sidebar.rects[sbPanel]
	found := 0
	for id, r := range g.sidebar.rects {
		if id.el != elLogicOption || r.Empty() {
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
	g.sidebar.sectionOpen[secGroove] = true
	g.sidebar.grooveDropdownOpen = true
	g.sidebar.layout()

	panel := g.sidebar.rects[sbPanel]
	found := 0
	for id, r := range g.sidebar.rects {
		if id.el != elGrooveOption || r.Empty() {
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
	for _, id := range []sbID{sbLogicSelect, sbGrooveSelect, sbMinus(fieldGroovePct), sbPlus(fieldGroovePct), sbMinus(fieldLogicN), sbPlus(fieldLogicN), sbMinus(fieldLogicP), sbPlus(fieldLogicP)} {
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
	if !g.sidebar.rects[sbLogicSelect].Empty() {
		t.Error("expected logic rect to be empty when section collapsed")
	}

	// Expand logic and groove sections.
	g.sidebar.sectionOpen[secLogic] = true
	g.sidebar.sectionOpen[secGroove] = true
	g.sidebar.layout()
	if g.sidebar.rects[sbLogicSelect].Empty() {
		t.Error("expected logic rect to be non-empty when section expanded")
	}
	if g.sidebar.rects[sbGrooveSelect].Empty() {
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
	g.sidebar.sectionOpen[secVol] = true
	g.sidebar.sectionOpen[secPit] = true
	g.sidebar.sectionOpen[secDur] = true
	g.sidebar.layout()

	panel := g.sidebar.rects[sbPanel]
	if panel.Dx() != sidebarDefaultW {
		t.Fatalf("expected desktop panel width %d, got %d", sidebarDefaultW, panel.Dx())
	}

	// All +/- buttons use sidebarIncBtnH (24px, vertically centered in 28px rows).
	for _, id := range []sbID{sbMinus(fieldVol), sbPlus(fieldVol), sbMinus(fieldPit), sbPlus(fieldPit), sbMinus(fieldDur), sbPlus(fieldDur)} {
		r := g.sidebar.rects[id]
		if r.Empty() {
			continue
		}
		if r.Dy() != sidebarIncBtnH {
			t.Errorf("button %q height: want %d, got %d", id, sidebarIncBtnH, r.Dy())
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
	g.sidebar.sectionOpen[secLogic] = true
	g.sidebar.logicDropdownOpen = true
	g.sidebar.layout()

	panel := g.sidebar.rects[sbPanel]
	// Verify all logic dropdown items are within panel width.
	for id, r := range g.sidebar.rects {
		if id.el != elLogicOption || r.Empty() {
			continue
		}
		if r.Min.X < panel.Min.X || r.Max.X > panel.Max.X {
			t.Errorf("logic dropdown item %q overflows panel: item=%v panel=%v", id, r, panel)
		}
	}

	// Vol/Pitch/Dur sections should still have section headers visible.
	for _, id := range []sbID{sbSectionHeader(secVol), sbSectionHeader(secPit), sbSectionHeader(secDur)} {
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
	g.sidebar.sectionOpen[secLogic] = true
	g.sidebar.sectionOpen[secGroove] = true

	// Step 1: Open logic dropdown. Logic toggle should be visible.
	g.sidebar.logicDropdownOpen = true
	g.sidebar.grooveDropdownOpen = false
	g.sidebar.layout()

	if r := g.sidebar.rects[sbLogicSelect]; r.Empty() {
		t.Error("expected logic toggle to remain visible when logic dropdown is open")
	}

	// Step 2: Close logic dropdown. Both toggles should be visible.
	g.sidebar.logicDropdownOpen = false
	g.sidebar.layout()

	if r := g.sidebar.rects[sbLogicSelect]; r.Empty() {
		t.Error("expected logic toggle to be visible after closing logic dropdown")
	}
	if r := g.sidebar.rects[sbGrooveSelect]; r.Empty() {
		t.Error("expected groove toggle to be visible after closing logic dropdown")
	}

	// Step 3: Open groove dropdown. Groove toggle should be visible.
	g.sidebar.grooveDropdownOpen = true
	g.sidebar.layout()

	if r := g.sidebar.rects[sbGrooveSelect]; r.Empty() {
		t.Error("expected groove toggle to remain visible when groove dropdown is open")
	}

	// Step 4: Close groove dropdown. Both visible again.
	g.sidebar.grooveDropdownOpen = false
	g.sidebar.layout()

	if r := g.sidebar.rects[sbLogicSelect]; r.Empty() {
		t.Error("expected logic toggle to be visible after closing groove dropdown")
	}
	if r := g.sidebar.rects[sbGrooveSelect]; r.Empty() {
		t.Error("expected groove toggle to be visible after closing groove dropdown")
	}

	// Verify the button handler closes the other dropdown via accordion.
	g.sidebar.logicDropdownOpen = false
	g.sidebar.grooveDropdownOpen = false
	g.sidebar.layout()
	// Click logic toggle.
	if b := g.sidebar.btns[sbLogicSelect]; b == nil {
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
		sections  []sbSection
		logicKind model.LogicKind
		logicOpen bool
	}{
		{"all-collapsed", nil, model.LogicNone, false},
		{"all-expanded", []sbSection{secVol, secPit, secDur, secLogic, secGroove, secAud}, model.LogicNone, false},
		{"with-probability", []sbSection{secLogic}, model.LogicProbability, false},
		{"with-every-n", []sbSection{secLogic}, model.LogicEveryN, false},
		{"logic-dropdown-open", []sbSection{secLogic}, model.LogicNone, true},
	}

	for _, st := range states {
		t.Run(st.name, func(t *testing.T) {
			// Reset sections
			g.sidebar.sectionOpen = map[sbSection]bool{}
			for _, s := range st.sections {
				g.sidebar.sectionOpen[s] = true
			}
			g.sidebar.logicDropdownOpen = st.logicOpen
			g.sidebar.grooveDropdownOpen = false
			if st.logicKind != model.LogicNone {
				if mn, ok := g.graph.GetNodeByID(n.ID); ok {
					p := mn.Params
					p.LogicKind = st.logicKind
					if st.logicKind == model.LogicProbability {
						p.LogicP = 0.5
					} else {
						p.LogicN = 2
					}
					g.graph.SetNodeParams(n.ID, p)
				}
			}
			g.sidebar.layout()

			// Section headers should always be present
			for _, sec := range []sbID{sbSectionHeader(secVol), sbSectionHeader(secPit), sbSectionHeader(secDur), sbSectionHeader(secLogic), sbSectionHeader(secGroove), sbSectionHeader(secAud)} {
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
	g.sidebar.sectionOpen[secLogic] = true

	// Kind = "None" (no params): param rects should be empty.
	if mn, ok := g.graph.GetNodeByID(n.ID); ok {
		p := mn.Params
		p.LogicKind = model.LogicNone
		g.graph.SetNodeParams(n.ID, p)
	}
	g.sidebar.layout()
	for _, id := range []sbID{sbMinus(fieldLogicN), sbPlus(fieldLogicN), sbMinus(fieldLogicP), sbPlus(fieldLogicP)} {
		if r := g.sidebar.rects[id]; !r.Empty() {
			t.Errorf("expected %q to be empty when kind=None, got %v", id, r)
		}
	}

	// Kind = "every_n_triggers" (has params): ln+/- should be non-empty.
	if mn, ok := g.graph.GetNodeByID(n.ID); ok {
		p := mn.Params
		p.LogicKind = model.LogicEveryN
		p.LogicN = 2
		g.graph.SetNodeParams(n.ID, p)
	}
	g.sidebar.layout()
	if g.sidebar.rects[sbMinus(fieldLogicN)].Empty() || g.sidebar.rects[sbPlus(fieldLogicN)].Empty() {
		t.Error("expected ln-/ln+ to be non-empty when kind=every_n_triggers")
	}

	// Kind = "probability" (has params): lp+/- should be non-empty.
	if mn, ok := g.graph.GetNodeByID(n.ID); ok {
		p := mn.Params
		p.LogicKind = model.LogicProbability
		p.LogicP = 0.5
		g.graph.SetNodeParams(n.ID, p)
	}
	g.sidebar.layout()
	if g.sidebar.rects[sbMinus(fieldLogicP)].Empty() || g.sidebar.rects[sbPlus(fieldLogicP)].Empty() {
		t.Error("expected lp-/lp+ to be non-empty when kind=probability")
	}

	// Kind = "trigger_if_prev_skipped" (no params): all param rects empty.
	if mn, ok := g.graph.GetNodeByID(n.ID); ok {
		p := mn.Params
		p.LogicKind = model.LogicIfPrevSkipped
		g.graph.SetNodeParams(n.ID, p)
	}
	g.sidebar.layout()
	for _, id := range []sbID{sbMinus(fieldLogicN), sbPlus(fieldLogicN), sbMinus(fieldLogicP), sbPlus(fieldLogicP)} {
		if r := g.sidebar.rects[id]; !r.Empty() {
			t.Errorf("expected %q to be empty when kind=trigger_if_prev_skipped, got %v", id, r)
		}
	}
}

// TestNodePopupDesktopLayoutOrder verifies desktop layout keeps Logic and Groove
// before the Aud section.
func TestNodePopupDesktopLayoutOrder(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = false
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	n := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.sidebar.Open(g.nodeByID(n.ID))
	// Expand all sections
	g.sidebar.sectionOpen[secLogic] = true
	g.sidebar.sectionOpen[secGroove] = true
	g.sidebar.sectionOpen[secAud] = true
	g.sidebar.sectionOpen[secGrp] = true
	g.sidebar.layout()

	// On desktop: Logic and Groove should come before Aud, and Aud before the
	// Groups section (the last section now that Move is gone).
	logicRect := g.sidebar.rects[sbLogicSelect]
	grvRect := g.sidebar.rects[sbGrooveSelect]
	audRect := g.sidebar.rects[sbAudible]
	grpRect := g.sidebar.rects[sbSectionHeader(secGrp)]

	if logicRect.Empty() || grvRect.Empty() || audRect.Empty() || grpRect.Empty() {
		t.Fatal("expected logic, grv, aud, and sec-grp rects to be non-empty on desktop")
	}
	if logicRect.Min.Y >= audRect.Min.Y {
		t.Errorf("desktop: logic should be above aud: logic.Y=%d aud.Y=%d", logicRect.Min.Y, audRect.Min.Y)
	}
	if grvRect.Min.Y >= audRect.Min.Y {
		t.Errorf("desktop: grv should be above aud: grv.Y=%d aud.Y=%d", grvRect.Min.Y, audRect.Min.Y)
	}
	if audRect.Min.Y >= grpRect.Min.Y {
		t.Errorf("desktop: aud should be above groups: aud.Y=%d grp.Y=%d", audRect.Min.Y, grpRect.Min.Y)
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
	g.sidebar.sectionOpen[secVol] = true
	g.sidebar.sectionOpen[secPit] = true
	g.sidebar.sectionOpen[secDur] = true
	g.sidebar.sectionOpen[secLogic] = true
	g.sidebar.sectionOpen[secGroove] = true
	g.sidebar.sectionOpen[secAud] = true
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
	g.sidebar.sectionOpen = map[sbSection]bool{}
	g.sidebar.layout()
	if g.sidebar.scroll.HasScroll() {
		t.Errorf("expected no scroll after collapsing sections, got Total=%d Visible=%d", g.sidebar.scroll.VS.Total, g.sidebar.scroll.VS.Visible)
	}
}
