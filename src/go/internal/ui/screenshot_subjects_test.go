package ui

import (
	"image"
	"testing"
)

// TestSubjectByName covers the symmetric parse of the Subject string form,
// including the empty-string case that maps to SubjectFullScreen.
func TestSubjectByName(t *testing.T) {
	if s, ok := SubjectByName(""); !ok || s != SubjectFullScreen {
		t.Fatalf(`SubjectByName("") = %q, %v; want SubjectFullScreen, true`, s, ok)
	}
	for _, want := range AllSubjects() {
		got, ok := SubjectByName(string(want))
		if !ok || got != want {
			t.Errorf("SubjectByName(%q) = %q, %v; want %q, true", want, got, ok, want)
		}
	}
	if _, ok := SubjectByName("nope"); ok {
		t.Error(`SubjectByName("nope") = ok=true; want ok=false`)
	}
}

// TestSubjectRectFullScreen — SubjectFullScreen always returns the entire
// framebuffer rect and ok=true.
func TestSubjectRectFullScreen(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	r, ok := g.SubjectRect(SubjectFullScreen)
	if !ok {
		t.Fatal("SubjectFullScreen returned ok=false")
	}
	if want := image.Rect(0, 0, 1280, 720); r != want {
		t.Errorf("SubjectFullScreen rect = %v; want %v", r, want)
	}
}

// TestSubjectRectAlwaysVisible — surfaces that exist on every frame
// (MainGrid, DrumView, EQ panel/tabs, Toolbar) must return non-empty rects
// inside the framebuffer after one Layout+Update cycle.
func TestSubjectRectAlwaysVisible(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	_ = g.Update()

	full := image.Rect(0, 0, 1280, 720)
	for _, s := range []Subject{
		SubjectMainGrid,
		SubjectDrumView,
		SubjectEQPanel,
		SubjectEQTabEQ,
		SubjectEQTabWave,
		SubjectEQTabSpectrum,
		SubjectEQTabLevels,
		SubjectToolbar,
	} {
		r, ok := g.SubjectRect(s)
		if !ok {
			t.Errorf("SubjectRect(%q) ok=false; expected always-visible surface", s)
			continue
		}
		if r.Empty() {
			t.Errorf("SubjectRect(%q) empty rect %v", s, r)
		}
		if !r.In(full) {
			t.Errorf("SubjectRect(%q) = %v escapes framebuffer %v", s, r, full)
		}
	}
}

// TestSubjectRectGatedByVisibility — surfaces tied to popup state report
// ok=false when closed and ok=true with a non-empty rect once their scene
// is applied.
func TestSubjectRectGatedByVisibility(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	_ = g.Update()

	// Closed initially.
	for _, s := range []Subject{
		SubjectFXPanel,
		SubjectContextMenu,
		SubjectOverflowMenu,
		SubjectInstrumentMenu,
	} {
		if _, ok := g.SubjectRect(s); ok {
			t.Errorf("SubjectRect(%q) ok=true with all popups closed; want false", s)
		}
	}

	// Context menu — open via the same path scene_catalog uses.
	ensureRow(g, 0)
	g.drum.OpenContextMenu(0)
	_ = g.Update()
	if r, ok := g.SubjectRect(SubjectContextMenu); !ok || r.Empty() {
		t.Errorf("SubjectContextMenu after OpenContextMenu = %v, ok=%v; want non-empty, true", r, ok)
	}
	g.drum.CloseContextMenu()
	_ = g.Update()
	if _, ok := g.SubjectRect(SubjectContextMenu); ok {
		t.Error("SubjectContextMenu still ok=true after CloseContextMenu")
	}

	// Instrument menu.
	g.drum.OpenInstrumentMenu(0)
	_ = g.Update()
	if r, ok := g.SubjectRect(SubjectInstrumentMenu); !ok || r.Empty() {
		t.Errorf("SubjectInstrumentMenu after OpenInstrumentMenu = %v, ok=%v; want non-empty, true", r, ok)
	}
	g.drum.CloseInstrumentMenu()
	_ = g.Update()

	// FX panel.
	g.drum.OpenFXPanel(0)
	_ = g.Update()
	if r, ok := g.SubjectRect(SubjectFXPanel); !ok || r.Empty() {
		t.Errorf("SubjectFXPanel after OpenFXPanel = %v, ok=%v; want non-empty, true", r, ok)
	}
	g.drum.CloseFXPanel()
	_ = g.Update()
	if _, ok := g.SubjectRect(SubjectFXPanel); ok {
		t.Error("SubjectFXPanel still ok=true after CloseFXPanel")
	}
}

// TestSubjectRectScopeWhenTabActive — scope rect is only meaningful once
// the EQ panel's Scope tab is selected; before that the scope zone has no
// laid-out rect and should report ok=false.
func TestSubjectRectScopeWhenTabActive(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	_ = g.Update()

	// Activate scope tab — this is the same path the eq_tab_scope catalog
	// scene takes.
	if err := g.SetActiveEQTab("scope"); err != nil {
		t.Fatalf("SetActiveEQTab(scope): %v", err)
	}
	g.SetChainVisible(true)
	for i := 0; i < 5; i++ {
		_ = g.Update()
	}

	r, ok := g.SubjectRect(SubjectChain)
	if !ok {
		t.Fatal("SubjectChain ok=false after activating Scope tab")
	}
	if r.Empty() {
		t.Errorf("SubjectChain returned empty rect after activation")
	}
	full := image.Rect(0, 0, 1280, 720)
	if !r.In(full) {
		t.Errorf("SubjectChain rect %v escapes framebuffer %v", r, full)
	}
}

// TestSubjectRectClampsToFramebuffer — even if a subject's underlying rect
// is somehow larger than the screen, SubjectRect must clip to the
// framebuffer (the screenshot encoder relies on this invariant to never
// receive an out-of-bounds sub-image rectangle).
func TestSubjectRectClampsToFramebuffer(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	_ = g.Update()

	full := image.Rect(0, 0, 800, 600)
	for _, s := range AllSubjects() {
		r, ok := g.SubjectRect(s)
		if !ok {
			continue
		}
		if !r.In(full) {
			t.Errorf("SubjectRect(%q) = %v escapes framebuffer %v", s, r, full)
		}
	}
}

// TestSubjectRectEmptyGameSafe — calling SubjectRect on a Game whose
// Layout has not been driven yet must not panic; every subject either
// returns ok=false or a degenerate rect, never a half-initialized rect.
func TestSubjectRectEmptyGameSafe(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	// Deliberately no Layout / Update.

	for _, s := range AllSubjects() {
		// Only requirement: this must not panic.
		_, _ = g.SubjectRect(s)
	}
}
