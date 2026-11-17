package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// The Chain tab is a developer tool: the production UI (beatmo.io, or a
// developer build with SetDevToolsForcedOff) must not show it and must refuse
// every route into it — pills, the number keys, the JS/scene export, the
// mobile view-mode switch — leaving the current tab active (never a blank or
// hidden panel). The Chain code stays compiled in; only the view is hidden.

func TestProductionHidesTheChainTab(t *testing.T) {
	forceProductionUIForTest(t)
	g := newTestGameForUndo(t)

	for _, tab := range AllPanelTabs() {
		if tab == TabScope {
			t.Fatal("AllPanelTabs lists the Chain tab in the production UI")
		}
	}
	chainLabel := PanelTabLabel(TabScope)
	for i, b := range g.drum.eqPanelZone.stickyBar.tabBtns {
		if b != nil && b.Text == chainLabel {
			t.Fatalf("sticky-bar pill %d is labelled %q in the production UI", i, chainLabel)
		}
	}

	for k := ebiten.Key1; k <= ebiten.Key7; k++ {
		restore := stubKeys(nil, map[ebiten.Key]bool{k: true})
		g.handleGlobalShortcuts()
		restore()
		if got := g.drum.eqPanelZone.tabState.ActiveTab(); got == TabScope {
			t.Fatalf("number key %v selected the Chain tab in the production UI", k)
		}
	}

	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	routes := map[string]func(){
		"SetActiveEQTab(chain)":      func() { _ = g.SetActiveEQTab("chain") },
		"SetChainVisible":            func() { g.drum.SetChainVisible(true) },
		"EQPanelZone.SetActiveTab":   func() { g.drum.eqPanelZone.SetActiveTab(TabScope) },
		"PanelTabState.SetActiveTab": func() { g.drum.eqPanelZone.tabState.SetActiveTab(TabScope) },
		"setViewMode(chain)":         func() { g.drum.setViewMode(viewModeChain) },
	}
	for name, route := range routes {
		route()
		if got := g.drum.eqPanelZone.tabState.ActiveTab(); got != TabSynth {
			t.Errorf("%s: active tab = %v, want the current tab (Synth) kept", name, got)
		}
		if g.drum.currentViewMode == viewModeChain {
			t.Errorf("%s: view mode switched to Chain in the production UI", name)
		}
	}
	if err := g.SetActiveEQTab("chain"); err == nil {
		t.Error(`SetActiveEQTab("chain") must report that the tab is unavailable`)
	}

	for _, m := range bottomNavModes() {
		if m == viewModeChain {
			t.Fatal("the bottom nav lists the Chain view in the production UI")
		}
	}
	labels := g.drum.bottomNavLabels()
	if len(labels) != len(bottomNavModes()) {
		t.Fatalf("bottom-nav labels (%d) and modes (%d) disagree", len(labels), len(bottomNavModes()))
	}
	for _, l := range labels {
		if l == PanelTabLabelForProfile(TabScope) || l == "Chn" {
			t.Fatalf("bottom-nav label %q in the production UI", l)
		}
	}
}

func TestDevBuildShowsTheChainTab(t *testing.T) {
	if !devToolsBuild {
		t.Skip("release build")
	}
	g := newTestGameForUndo(t)
	found := false
	for _, tab := range AllPanelTabs() {
		found = found || tab == TabScope
	}
	if !found {
		t.Fatal("a developer build must list the Chain tab")
	}
	if err := g.SetActiveEQTab("chain"); err != nil {
		t.Fatal(err)
	}
	if got := g.drum.eqPanelZone.tabState.ActiveTab(); got != TabScope {
		t.Fatalf("developer build: SetActiveEQTab(chain) left %v active", got)
	}
	navHasChain := false
	for _, m := range bottomNavModes() {
		navHasChain = navHasChain || m == viewModeChain
	}
	if !navHasChain {
		t.Fatal("a developer build's bottom nav must list the Chain view")
	}
}
