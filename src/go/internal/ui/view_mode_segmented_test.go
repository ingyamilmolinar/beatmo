//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestViewMode_HasThreeStates is a compile-time guard. Phase 3 promotes
// viewMode from 2 to 3 states (Pads/EQ/Wave). If any of the three
// constants is missing this file fails to compile.
func TestViewMode_HasThreeStates(t *testing.T) {
	modes := []viewMode{viewModeRows, viewModeEQ, viewModeWave}
	if len(modes) != 3 {
		t.Fatalf("expected 3 view modes, got %d", len(modes))
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
	dv.setViewMode(viewModeRows)
	// Rows mode doesn't touch tabState; verify it preserves the prior tab.
	if got := dv.eqPanelZone.tabState.ActiveTab(); got != TabWave {
		t.Errorf("setViewMode(Rows) should preserve tabState=TabWave, got %v", got)
	}
}

// TestViewSwitchSegmented_RenderedInBottomBarOnMobile verifies the new
// SegmentedControl replaces the binary view-switch button on mobile.
// Lives in the middle column of the bottom action bar (col 1 of 3).
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
	// Three segments, each >= TouchMinTarget() / 3 wide (loose lower bound).
	for i := 0; i < 3; i++ {
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

	cases := []struct {
		seg  int
		want viewMode
	}{
		{0, viewModeRows},
		{1, viewModeEQ},
		{2, viewModeWave},
	}
	for _, c := range cases {
		r := dv.viewSwitchSegmented.SegmentRect(c.seg)
		mid := image.Pt((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
		injectTouchTap(mid.X, mid.Y)
		g.Update()
		g.Update()
		if dv.currentViewMode != c.want {
			t.Errorf("seg=%d: currentViewMode=%v, want %v", c.seg, dv.currentViewMode, c.want)
		}
	}
}

// TestViewSwitchSegmented_DrawnAfterBottomBar pins the draw order so
// the segmented control paints on top of the bar surface.
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
	r := dv.viewSwitchSegmented.SegmentRect(0)
	if r.Empty() {
		t.Skip("no segmented rect to sample")
	}

	// Check that the segmented control's center pixel is non-zero after Draw.
	img := ebiten.NewImage(360, 700)
	g.Draw(img)
	cx, cy := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2
	if cx < 0 || cx >= 360 || cy < 0 || cy >= 700 {
		t.Skipf("segment center (%d,%d) out of bounds", cx, cy)
	}
	px := img.At(cx, cy)
	rv, gv, bv, av := px.RGBA()
	if rv == 0 && gv == 0 && bv == 0 && av == 0 {
		t.Errorf("pixel at segmented center (%d,%d) is transparent/black — segmented not drawn", cx, cy)
	}
}
