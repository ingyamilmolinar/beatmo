//go:build test

package ui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// The tab system owns the drum-view layout: currentViewMode is the ONLY source of which zones paint on mobile,
// mobileEQMode is derived from it, and EQPanelZone is hidden by the tree's visibility gate rather than its own
// Draw guard.

// findLayerByID returns the registered layer with the given ID, or nil.
func findLayerByID(t *testing.T, dv *DrumView, id string) Layer {
	t.Helper()
	if dv.tree == nil {
		t.Fatalf("dv.tree nil")
	}
	// The audio-panel layers live in dv.audioTree, everything else in dv.tree.
	for _, l := range dv.tree.LayersForTest() {
		if l.ID() == id {
			return l
		}
	}
	if dv.audioTree != nil {
		for _, l := range dv.audioTree.LayersForTest() {
			if l.ID() == id {
				return l
			}
		}
	}
	return nil
}

func TestMobile_PadsMode_HidesEQPanelZone_InTree(t *testing.T) {
	setupMobileTest(t, true)

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)
	advanceFrames(g, 2)
	dv := g.drum

	// First enter EQ so the zone has been laid out and would draw if visible.
	dv.setViewMode(viewModeEQ)
	advanceFrames(g, 2)
	if eqL := findLayerByID(t, dv, "eq-panel"); eqL == nil || !eqL.Visible() {
		t.Fatalf("precondition: eq-panel layer should be Visible() in viewModeEQ on mobile")
	}

	// Switch back to Pads. Tree must hide the EQ panel.
	dv.setViewMode(viewModeRows)
	advanceFrames(g, 2)

	eqL := findLayerByID(t, dv, "eq-panel")
	if eqL == nil {
		t.Fatalf("eq-panel layer not registered in tree")
	}
	if eqL.Visible() {
		t.Errorf("mobile + viewModeRows: eq-panel layer should be hidden by tree, got Visible()=true")
	}
}

// Desktop has no Pads/Audio swap: the panel sits beside the rows in every mode.
func TestDesktop_AllModesShowEQPanel(t *testing.T) {
	assertDefaultParityState(t)

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 800)
	advanceFrames(g, 2)
	dv := g.drum

	for _, m := range []viewMode{viewModeRows, viewModeEQ, viewModeWave, viewModeSpectrum, viewModeMeters, viewModeChain} {
		dv.setViewMode(m)
		advanceFrames(g, 1)
		eqL := findLayerByID(t, dv, "eq-panel")
		if eqL == nil {
			t.Fatalf("eq-panel layer not registered (desktop)")
		}
		if !eqL.Visible() {
			t.Errorf("desktop viewMode=%v: eq-panel must remain Visible(), got false", m)
		}
	}
}

// A counter, not pixels: layout collapses the panel to zero height in tests, where Draw's own small-rect guard
// hides a leak that a real, larger viewport would show. Only the tree gate guarantees it never paints.
func TestMobile_PadsAfterEQ_EQPanelDrawNotInvoked(t *testing.T) {
	setupMobileTest(t, true)

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	// Portrait (mobile landscape replaces the UI with a notice), tall enough that the EQ widget keeps a non-zero rect
	// and the tree gate, not the zero-rect guard, is exercised.
	g.Layout(430, 720)
	advanceFrames(g, 2)
	dv := g.drum

	// Visit the EQ tab so the zone is known-good and known-paintable.
	dv.setViewMode(viewModeEQ)
	advanceFrames(g, 2)
	g.Draw(ebiten.NewImage(430, 720))
	if dv.eqPanelZone.DrawCallsForTest() == 0 {
		t.Fatalf("precondition: EQPanelZone.Draw should have been invoked at least once during viewModeEQ")
	}

	// The tree's visibility gate must short-circuit Draw at dispatch.
	dv.setViewMode(viewModeRows)
	advanceFrames(g, 2)
	before := dv.eqPanelZone.DrawCallsForTest()
	g.Draw(ebiten.NewImage(430, 720))
	after := dv.eqPanelZone.DrawCallsForTest()
	if delta := after - before; delta != 0 {
		t.Errorf("mobile Pads after EQ: EQPanelZone.Draw was invoked %d times during a frame in viewModeRows — pills will leak. The tree's RegisterZoneVisible gate is not short-circuiting the zone.", delta)
	}
}

func TestMobileEQMode_DerivedFromCurrentViewMode(t *testing.T) {
	setupMobileTest(t, true)

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)
	advanceFrames(g, 2)
	dv := g.drum

	dv.setViewMode(viewModeRows)
	if dv.MobileEQMode() {
		t.Errorf("after setViewMode(Rows): MobileEQMode()=true, want false")
	}

	for _, m := range []viewMode{viewModeEQ, viewModeWave, viewModeSpectrum, viewModeMeters, viewModeChain} {
		dv.setViewMode(m)
		if !dv.MobileEQMode() {
			t.Errorf("after setViewMode(%v): MobileEQMode()=false, want true", m)
		}
	}

	// Desktop: MobileEQMode() must always be false even with audio viewMode.
	g2 := New(logger)
	t.Cleanup(g2.CloseForTest)
	// Restore non-mobile profile for this sub-check.
	prev := forceSmallScreenForTest
	forceSmallScreenForTest = false
	t.Cleanup(func() { forceSmallScreenForTest = prev })
	g2.Layout(1280, 800)
	advanceFrames(g2, 2)
	dv2 := g2.drum
	dv2.setViewMode(viewModeEQ)
	if dv2.MobileEQMode() {
		t.Errorf("desktop after setViewMode(EQ): MobileEQMode()=true, want false")
	}
}

// currentViewMode, the segmented control and tabState change in lockstep, as on any tab switch.
func TestSetMobileEQMode_RoutesThroughSetViewMode(t *testing.T) {
	setupMobileTest(t, true)

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)
	advanceFrames(g, 2)
	dv := g.drum

	dv.setViewMode(viewModeRows)
	dv.SetMobileEQMode(true)
	if dv.currentViewMode != viewModeEQ {
		t.Errorf("after SetMobileEQMode(true) from Rows: currentViewMode=%v, want viewModeEQ", dv.currentViewMode)
	}
	if dv.viewSwitchSegmented != nil && dv.viewSwitchSegmented.Active() != 1 {
		t.Errorf("after SetMobileEQMode(true): segmented active=%d, want 1", dv.viewSwitchSegmented.Active())
	}

	// And SetMobileEQMode(false) returns to viewModeRows.
	dv.SetMobileEQMode(false)
	if dv.currentViewMode != viewModeRows {
		t.Errorf("after SetMobileEQMode(false): currentViewMode=%v, want viewModeRows", dv.currentViewMode)
	}
	if dv.viewSwitchSegmented != nil && dv.viewSwitchSegmented.Active() != 0 {
		t.Errorf("after SetMobileEQMode(false): segmented active=%d, want 0", dv.viewSwitchSegmented.Active())
	}
}

// Only the owner files may assign currentViewMode or mobileEQMode: the tab system owns the swap.
func TestNoAdHocViewModeMutation(t *testing.T) {
	allowedWriters := map[string]bool{
		"drumview_context_menu.go": true,
		"drumview.go":              true,
	}
	forbiddenLHS := map[string]bool{
		"currentViewMode": true,
		"mobileEQMode":    true,
	}

	matches, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	fset := token.NewFileSet()
	for _, path := range matches {
		base := filepath.Base(path)
		if strings.HasSuffix(base, "_test.go") {
			continue
		}
		if allowedWriters[base] {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			as, ok := n.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for _, lhs := range as.Lhs {
				sel, ok := lhs.(*ast.SelectorExpr)
				if !ok {
					continue
				}
				if forbiddenLHS[sel.Sel.Name] {
					pos := fset.Position(sel.Pos())
					t.Errorf("%s:%d: ad-hoc assignment to %q — only setViewMode (drumview_context_menu.go) may mutate this field; route through dv.setViewMode(target) instead",
						filepath.Base(pos.Filename), pos.Line, sel.Sel.Name)
				}
			}
			return true
		})
	}
}
