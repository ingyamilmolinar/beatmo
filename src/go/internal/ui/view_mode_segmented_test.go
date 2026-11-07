//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// A missing view-mode constant fails the build.
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
	// Only side padding: the bar holds the segmented control alone.
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

	// SetInputForTest resets inputForTestActive itself, while injectTouchTap is suppressed when a previous test left it
	// set.
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
		// Master blocks Synth/Sampler; selecting a row's instrument enables the segment.
		if c.want == viewModeSynth || c.want == viewModeSampler {
			if len(dv.Rows) > 0 {
				dv.eqPanelZone.SetActiveChannel(channel.Of(dv.Rows[0].Instrument))
			}
		}
		r := dv.viewSwitchSegmented.SegmentRect(c.seg)
		mid := image.Pt((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
		simulateTap(mid.X, mid.Y)
		if dv.currentViewMode != c.want {
			t.Errorf("seg=%d: currentViewMode=%v, want %v", c.seg, dv.currentViewMode, c.want)
		}
	}
}

// The stub writes no real pixels, so drawRoundedRect calls are intercepted.
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
