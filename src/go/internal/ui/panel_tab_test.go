//go:build test

package ui

import "testing"

func TestPanelTabDefault(t *testing.T) {
	assertDefaultParityState(t)

	s := NewPanelTabState()
	if s.ActiveTab() != TabEQ {
		t.Fatalf("expected default tab TabEQ (%d), got %d", TabEQ, s.ActiveTab())
	}
	if s.Expanded() {
		t.Fatal("expected default state to be collapsed")
	}
}

func TestPanelTabSwitch(t *testing.T) {
	assertDefaultParityState(t)

	s := NewPanelTabState()
	s.SetActiveTab(TabWave)
	if s.ActiveTab() != TabWave {
		t.Fatalf("expected TabWave after SetActiveTab, got %d", s.ActiveTab())
	}
	s.SetActiveTab(TabSpectrum)
	if s.ActiveTab() != TabSpectrum {
		t.Fatalf("expected TabSpectrum after SetActiveTab, got %d", s.ActiveTab())
	}
	s.SetActiveTab(TabMeters)
	if s.ActiveTab() != TabMeters {
		t.Fatalf("expected TabMeters after SetActiveTab, got %d", s.ActiveTab())
	}
	s.SetActiveTab(TabEQ)
	if s.ActiveTab() != TabEQ {
		t.Fatalf("expected TabEQ after SetActiveTab, got %d", s.ActiveTab())
	}
}

func TestPanelTabExpand(t *testing.T) {
	assertDefaultParityState(t)

	s := NewPanelTabState()
	if s.Expanded() {
		t.Fatal("expected collapsed by default")
	}
	s.ToggleExpanded()
	if !s.Expanded() {
		t.Fatal("expected expanded after first toggle")
	}
	s.ToggleExpanded()
	if s.Expanded() {
		t.Fatal("expected collapsed after second toggle")
	}
}

func TestPanelTabCollapsedHeight(t *testing.T) {
	assertDefaultParityState(t)

	prev := eqPanelHeight
	eqPanelHeight = 190
	t.Cleanup(func() { eqPanelHeight = prev })

	s := NewPanelTabState()
	if h := s.PanelHeight(); h != 190 {
		t.Fatalf("collapsed height: expected 190, got %d", h)
	}
}

func TestPanelTabExpandedHeight(t *testing.T) {
	assertDefaultParityState(t)

	prev := eqPanelHeight
	eqPanelHeight = 190
	t.Cleanup(func() { eqPanelHeight = prev })

	s := NewPanelTabState()
	s.ToggleExpanded()
	if h := s.PanelHeight(); h != 380 {
		t.Fatalf("expanded height: expected 380, got %d", h)
	}
}

func TestPanelTabLabels(t *testing.T) {
	assertDefaultParityState(t)

	tabs := AllPanelTabs()
	if len(tabs) != 4 {
		t.Fatalf("expected 4 tabs, got %d", len(tabs))
	}

	expected := map[PanelTab]string{
		TabWave:     "Wave",
		TabSpectrum: "Spectrum",
		TabMeters:   "Meters",
		TabEQ:       "EQ",
	}
	for _, tab := range tabs {
		label := PanelTabLabel(tab)
		want, ok := expected[tab]
		if !ok {
			t.Fatalf("unexpected tab %d in AllPanelTabs()", tab)
		}
		if label != want {
			t.Errorf("PanelTabLabel(%d) = %q, want %q", tab, label, want)
		}
	}
}
