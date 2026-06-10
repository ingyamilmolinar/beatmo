//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestViewMode_HasSixStates is a compile-time guard. The mobile redesign
// promotes viewMode to six states (Pads/EQ/Wave/Spec/Mtr/Scope). If any
// constant is missing this file fails to compile.
func TestViewMode_HasSixStates(t *testing.T) {
	modes := []viewMode{
		viewModeRows,
		viewModeEQ,
		viewModeWave,
		viewModeSpectrum,
		viewModeMeters,
		viewModeChain,
	}
	if len(modes) != 6 {
		t.Fatalf("expected 6 view modes, got %d", len(modes))
	}
	// Ensure all distinct.
	for i := 0; i < len(modes); i++ {
		for j := i + 1; j < len(modes); j++ {
			if modes[i] == modes[j] {
				t.Fatalf("modes[%d] == modes[%d] (%v)", i, j, modes[i])
			}
		}
	}
}

// TestSetViewMode_SyncsAudioPanelTabState verifies that switching to
// viewModeEQ lands the audio panel on TabEQ, and viewModeWave lands on
// TabWave. Drives the mapping the SegmentedControl needs.
func TestSetViewMode_SyncsAudioPanelTabState(t *testing.T) {
	assertDefaultParityState(t)
	setupMobileTest(t, true)

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)
	advanceFrames(g, 2)
	dv := g.drum

	dv.setViewMode(viewModeEQ)
	if got := dv.eqPanelZone.tabState.ActiveTab(); got != TabEQ {
		t.Errorf("after setViewMode(EQ): tabState=%v, want TabEQ", got)
	}
	dv.setViewMode(viewModeWave)
	if got := dv.eqPanelZone.tabState.ActiveTab(); got != TabWave {
		t.Errorf("after setViewMode(Wave): tabState=%v, want TabWave", got)
	}
	dv.setViewMode(viewModeSpectrum)
	if got := dv.eqPanelZone.tabState.ActiveTab(); got != TabSpectrum {
		t.Errorf("after setViewMode(Spectrum): tabState=%v, want TabSpectrum", got)
	}
	dv.setViewMode(viewModeMeters)
	if got := dv.eqPanelZone.tabState.ActiveTab(); got != TabMeters {
		t.Errorf("after setViewMode(Meters): tabState=%v, want TabMeters", got)
	}
	dv.setViewMode(viewModeChain)
	if got := dv.eqPanelZone.tabState.ActiveTab(); got != TabScope {
		t.Errorf("after setViewMode(Scope): tabState=%v, want TabScope", got)
	}
	dv.setViewMode(viewModeRows)
	// Rows mode doesn't touch tabState; verify it preserves the prior tab.
	if got := dv.eqPanelZone.tabState.ActiveTab(); got != TabScope {
		t.Errorf("setViewMode(Rows) should preserve tabState=TabScope, got %v", got)
	}
}

// TestViewSwitchSegmented_RenderedInBottomBarOnMobile verifies the new
// 6-segment SegmentedControl replaces the binary view-switch button on
// mobile and spans the full bottom action bar width.
func TestViewSwitchSegmented_RenderedInBottomBarOnMobile(t *testing.T) {
	assertDefaultParityState(t)
	setupMobileTest(t, true)

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)
	advanceFrames(g, 2)
	dv := g.drum

	sc := dv.viewSwitchSegmented
	if sc == nil {
		t.Fatalf("dv.viewSwitchSegmented should be non-nil on mobile")
	}
	if sc.Rect().Empty() {
		t.Fatalf("viewSwitchSegmented Rect empty on mobile")
	}
	bar := dv.bottomActionBarRect
	if !sc.Rect().In(bar) {
		t.Fatalf("viewSwitchSegmented %v not contained in bottom bar %v", sc.Rect(), bar)
	}
	if sc.Rect().Dy() < TouchMinTarget() {
		t.Fatalf("segmented height %d below TouchMinTarget %d", sc.Rect().Dy(), TouchMinTarget())
	}
	// Bar is now segmented-only (Theme 1) — control should span ≥ 80 % of
	// the bar's interior, leaving only horizontal padding on the sides.
	if sc.Rect().Dx() < bar.Dx()*4/5 {
		t.Errorf("segmented width %d should span most of bar width %d", sc.Rect().Dx(), bar.Dx())
	}
	// Six segments, all non-empty.
	for i := 0; i < 6; i++ {
		if sc.SegmentRect(i).Empty() {
			t.Errorf("segment %d rect empty", i)
		}
	}
}

// TestViewSwitchSegmented_AbsentOnDesktop guards desktop unchanged.
func TestViewSwitchSegmented_AbsentOnDesktop(t *testing.T) {
	assertDefaultParityState(t)

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 800)
	advanceFrames(g, 2)
	dv := g.drum
	if dv.viewSwitchSegmented != nil && !dv.viewSwitchSegmented.Rect().Empty() {
		t.Fatalf("desktop should not render segmented view-switch, got rect %v", dv.viewSwitchSegmented.Rect())
	}
}

// TestViewSwitchSegmented_TapDispatchesViewMode verifies a tap on each
// segment switches DrumView.currentViewMode.
func TestViewSwitchSegmented_TapDispatchesViewMode(t *testing.T) {
	assertDefaultParityState(t)
	setupMobileTest(t, true)

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)
	advanceFrames(g, 2)
	dv := g.drum

	if dv.viewSwitchSegmented == nil {
		t.Fatalf("precondition: viewSwitchSegmented non-nil")
	}

	// Use SetInputForTest (cursor + mouse click) rather than injectTouchTap
	// so the test is robust against stale inputForTestActive from prior tests
	// that used SetInputForTest. injectTouchTap is suppressed when
	// inputForTestActive=true; SetInputForTest always works since it resets
	// inputForTestActive itself.
	simulateTap := func(mx, my int) {
		var px, py int
		var pressed bool
		restore := SetInputForTest(
			func() (int, int) { return px, py },
			func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
			func(ebiten.Key) bool { return false },
			func() []rune { return nil },
			func() (float64, float64) { return 0, 0 },
			func() (int, int) { return 360, 700 },
		)
		px, py = mx, my
		pressed = true
		g.Update()
		pressed = false
		g.Update()
		restore()
	}

	cases := []struct {
		seg  int
		want viewMode
	}{
		{0, viewModeRows},
		{1, viewModeEQ},
		{2, viewModeWave},
		{3, viewModeSpectrum},
		{4, viewModeMeters},
		{5, viewModeChain},
		{6, viewModeSynth},
		{7, viewModeSampler},
	}
	for _, c := range cases {
		r := dv.viewSwitchSegmented.SegmentRect(c.seg)
		mid := image.Pt((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
		simulateTap(mid.X, mid.Y)
		if dv.currentViewMode != c.want {
			t.Errorf("seg=%d: currentViewMode=%v, want %v", c.seg, dv.currentViewMode, c.want)
		}
	}
}

// TestViewSwitchSegmented_DrawnAfterBottomBar pins the draw order so
// the segmented control paints on top of the bar surface. Uses the
// drawCallRecorder to intercept drawRoundedRect calls (ebitenstub
// does not write real pixels, so raw img.At reads would always return 0).
func TestViewSwitchSegmented_DrawnAfterBottomBar(t *testing.T) {
	assertDefaultParityState(t)
	setupMobileTest(t, true)

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)
	advanceFrames(g, 2)
	dv := g.drum

	if dv.viewSwitchSegmented == nil {
		t.Fatalf("precondition: viewSwitchSegmented non-nil on mobile")
	}
	scRect := dv.viewSwitchSegmented.Rect()
	if scRect.Empty() {
		t.Skip("no segmented rect to check")
	}

	// Intercept drawRoundedRect calls during g.Draw and verify at least one
	// call lands within the segmented control's rect region.
	img := ebiten.NewImage(360, 700)
	var rec drawCallRecorder
	rec.record(t, func() {
		g.Draw(img)
	})
	hits := rec.inRegion(scRect)
	if len(hits) == 0 {
		t.Errorf("no drawRoundedRect/drawRect calls found in segmented rect %v — segmented not drawn", scRect)
	}
}
