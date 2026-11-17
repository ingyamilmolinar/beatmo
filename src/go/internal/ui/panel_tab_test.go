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

func TestPanelTabAlwaysTallHeight(t *testing.T) {
	assertDefaultParityState(t)

	prev := eqPanelHeight
	eqPanelHeight = 190
	t.Cleanup(func() { eqPanelHeight = prev })

	// Every tab returns the tall height regardless of expanded state —
	// the kid-friendly redesign defaults every audio tab to the same
	// readable size. Drag-resize via DrumView.eqH overrides downstream.
	// The Synth tab is the one exception: it packs a row of stage cards
	// (dial + caption + concept band per knob) and gets one extra
	// eqPanelHeight of vertical room (see PanelHeightAt).
	for _, tab := range AllPanelTabs() {
		s := NewPanelTabState()
		s.SetActiveTab(tab)
		want := eqPanelHeight * RuntimeProf().AudioPanelHeightMultiplier
		if tab == TabSynth {
			want += eqPanelHeight
		}
		if h := s.PanelHeight(); h != want {
			t.Errorf("tab %d height: expected %d, got %d", tab, want, h)
		}
		s.ToggleExpanded()
		if h := s.PanelHeight(); h != want {
			t.Errorf("tab %d expanded height: expected %d, got %d", tab, want, h)
		}
	}
}

func TestPanelTabLabels(t *testing.T) {
	assertDefaultParityState(t)

	tabs := AllPanelTabs()
	if len(tabs) != 7 {
		t.Fatalf("expected 7 tabs (EQ + Wave + Spectrum + Levels + Chain + Synth + Sampler), got %d", len(tabs))
	}

	// "Levels" replaces "Meters" (every consumer DAW uses Levels);
	// "Chain" replaces "Scope" (communicates pipeline-stage view);
	// "Synth" surfaces SynthRecipe params; "Sampler" is the WAV/synth-capture
	// sample editor.
	expected := map[PanelTab]string{
		TabWave:     "Wave",
		TabSpectrum: "Spectrum",
		TabMeters:   "Levels",
		TabEQ:       "EQ",
		TabScope:    "Chain",
		TabSynth:    "Synth",
		TabSampler:  "Sampler",
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

func TestPanelTabSlugCanonical(t *testing.T) {
	assertDefaultParityState(t)

	expected := map[PanelTab]string{
		TabWave:     "wave",
		TabSpectrum: "spectrum",
		TabMeters:   "levels",
		TabEQ:       "eq",
		TabScope:    "chain",
		TabSampler:  "sampler",
	}
	for tab, want := range expected {
		if got := PanelTabSlug(tab); got != want {
			t.Errorf("PanelTabSlug(%d) = %q, want %q", tab, got, want)
		}
	}
}

func TestPanelTabSamplerMobileLabel(t *testing.T) {
	assertDefaultParityState(t)

	// setupMobileTest forces the small-screen profile with proper cleanup so
	// Profile() resolves to mobile even though detectSmallScreen() would
	// otherwise report desktop in the test harness. (The desktop "Sampler"
	// label is covered by TestPanelTabLabels under the default profile.)
	setupMobileTest(t, true)
	UpdateProfile()
	if got := PanelTabLabelForProfile(TabSampler); got != "Smpl" {
		t.Errorf("mobile Sampler label = %q, want %q", got, "Smpl")
	}
}
