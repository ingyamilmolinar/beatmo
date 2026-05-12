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

// These tests pin the architectural invariant the user asked for:
//
//   "Going back and forth using the tab system should own the entirety of
//    the drum view layout and switch it not doing ad-hoc switches through
//    code."
//
// Concretely: dv.currentViewMode is the ONLY source of truth for which
// top-level zones the DrumViewTree paints on mobile. dv.mobileEQMode is a
// derived getter, never a parallel field. EQPanelZone is hidden by the
// tree's visibility gate (not by an internal Draw() guard) when the user
// is on the Pads tab. Switching Pads → EQ → Pads does not leak the EQ
// "Master / HP / LP" pill strip into the rows area.

// findLayerByID returns the registered layer with the given ID, or nil.
func findLayerByID(t *testing.T, dv *DrumView, id string) Layer {
	t.Helper()
	if dv.tree == nil {
		t.Fatalf("dv.tree nil")
	}
	for _, l := range dv.tree.LayersForTest() {
		if l.ID() == id {
			return l
		}
	}
	return nil
}

// TestMobile_PadsMode_HidesEQPanelZone_InTree is the architectural assertion.
// On mobile, the EQ panel zone must report Visible()=false from the tree's
// perspective when currentViewMode == viewModeRows.
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

// TestDesktop_AllModesShowEQPanel guards desktop against the same gate.
// Desktop has no Pads/Audio swap concept — the EQ panel coexists side-by-side
// with rows, so it must remain Visible() in every viewMode.
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

// TestMobile_PadsAfterEQ_EQPanelDrawNotInvoked is the strongest reproducer
// of the screenshot bug (screenshot.png 2026-05-10): in mobile Pads mode
// after a visit to the EQ tab, the EQ panel zone's Draw method MUST NOT
// be invoked — that's the only way the pills can leak. We assert this
// directly via the zone's drawCalls counter, which increments on every
// entry into Draw before any short-circuit.
//
// Why a counter rather than a pixel test: the layout system collapses the
// EQ widget to zero height in viewModeRows so the existing
// `if rect.Dy() < 8` guard inside Draw masks the leak from pixel observers
// in test viewports. In production the widget can retain a non-zero rect
// (peek mode, larger viewports), and only the tree-level visibility gate
// is sufficient to guarantee the zone never paints. The counter is
// layout-agnostic: it catches the bug regardless of where the panel rect
// happens to land.
func TestMobile_PadsAfterEQ_EQPanelDrawNotInvoked(t *testing.T) {
	setupMobileTest(t, true)

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(569, 430)
	advanceFrames(g, 2)
	dv := g.drum

	// Visit the EQ tab so the zone is known-good and known-paintable.
	dv.setViewMode(viewModeEQ)
	advanceFrames(g, 2)
	g.Draw(ebiten.NewImage(569, 430))
	if dv.eqPanelZone.DrawCallsForTest() == 0 {
		t.Fatalf("precondition: EQPanelZone.Draw should have been invoked at least once during viewModeEQ")
	}

	// Switch to Pads, snapshot the counter, draw a frame, and assert the
	// counter did not move — the tree's RegisterZoneVisible gate must
	// short-circuit Draw at the dispatch level.
	dv.setViewMode(viewModeRows)
	advanceFrames(g, 2)
	before := dv.eqPanelZone.DrawCallsForTest()
	g.Draw(ebiten.NewImage(569, 430))
	after := dv.eqPanelZone.DrawCallsForTest()
	if delta := after - before; delta != 0 {
		t.Errorf("mobile Pads after EQ: EQPanelZone.Draw was invoked %d times during a frame in viewModeRows — pills will leak. The tree's RegisterZoneVisible gate is not short-circuiting the zone.", delta)
	}
}

// TestMobileEQMode_DerivedFromCurrentViewMode pins the single source of
// truth: dv.MobileEQMode() must reflect dv.currentViewMode, never a
// separately-mutated parallel flag.
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

// TestSetMobileEQMode_RoutesThroughSetViewMode verifies that the legacy
// SetMobileEQMode helper goes through the canonical setViewMode entry
// point — i.e. flipping it must update currentViewMode AND segmented
// control AND tabState in lockstep, just like any other tab switch.
func TestSetMobileEQMode_RoutesThroughSetViewMode(t *testing.T) {
	setupMobileTest(t, true)

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)
	advanceFrames(g, 2)
	dv := g.drum

	// Start on Rows. SetMobileEQMode(true) should land on viewModeEQ AND
	// sync the segmented control to segment 1 (the EQ slot).
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

// TestNoAdHocViewModeMutation is the durable enforcer of the user's
// "tab system owns the swap, no ad-hoc switches" principle. It walks every
// non-test .go file in internal/ui and forbids assignments to either
// dv.currentViewMode or dv.mobileEQMode outside the canonical owner files.
//
// Canonical writers:
//   - drumview_context_menu.go — owns setViewMode and SetMobileEQMode.
//   - drumview.go              — owns the field declarations themselves.
//
// Any other production file that writes either field will fail this test.
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
