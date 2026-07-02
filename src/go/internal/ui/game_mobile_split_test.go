//go:build test

package ui

import "testing"

func mobileSplitGame(t *testing.T, w, h int, mode viewMode) *Game {
	t.Helper()
	g := newMobileSynthTabGameForTestSize(t, w, h)
	g.drum.setViewMode(mode)
	g.Layout(w, h)
	return g
}

func TestMobileSplit_GridAtLeast40PercentAllTabs(t *testing.T) {
	modes := []viewMode{viewModeRows, viewModeSynth, viewModeSampler, viewModeEQ, viewModeWave, viewModeSpectrum, viewModeMeters, viewModeChain}
	for _, h := range []int{720, 844} {
		for _, mode := range modes {
			g := mobileSplitGame(t, 390, h, mode)
			y := adaptiveMobilePortraitSplitY(h, g)
			if y < h*40/100 {
				t.Errorf("h=%d mode=%v: grid %d < 40%% of %d", h, mode, y, h)
			}
			if y <= 0 {
				t.Errorf("h=%d mode=%v: grid not visible", h, mode)
			}
		}
	}
}

func TestMobileSplit_DoesNotOverExpand(t *testing.T) {
	h := 1280
	g := mobileSplitGame(t, 390, h, viewModeSynth)
	y := adaptiveMobilePortraitSplitY(h, g)
	drumH := h - y
	audioNeed := Profile().HeaderMaxH + mobileAudioPanelMinContentH() + TouchMinTarget()
	// snap may add up to one row to align the rack
	if drumH > audioNeed+TouchRowHeight()+1 {
		t.Errorf("drum pane %d over-expanded past content need %d", drumH, audioNeed)
	}
	if y <= h*40/100 {
		t.Errorf("tall viewport: grid %d should exceed 40%% (%d)", y, h*40/100)
	}
}

func TestMobileSplit_StableAcrossTabs(t *testing.T) {
	modes := []viewMode{viewModeRows, viewModeSynth, viewModeSampler, viewModeEQ, viewModeWave, viewModeSpectrum, viewModeMeters, viewModeChain}
	for _, h := range []int{720, 844} {
		var first int
		for i, mode := range modes {
			g := mobileSplitGame(t, 390, h, mode)
			y := adaptiveMobilePortraitSplitY(h, g)
			if i == 0 {
				first = y
				continue
			}
			if y != first {
				t.Errorf("h=%d: split differs by tab — %v gives %d, want %d (stable across tabs)", h, mode, y, first)
			}
		}
	}
}

func TestMobileSplit_AppliedAtStartupAndOnResize(t *testing.T) {
	g := newMobileSynthTabGameForTestSize(t, 390, 844)
	want844 := adaptiveMobilePortraitSplitY(844, g)
	if g.split.Y != want844 {
		t.Fatalf("startup split.Y=%d, want stable %d", g.split.Y, want844)
	}
	if g.split.Y == 844/2 {
		t.Fatalf("split still default 0.5 (%d) — not applied at startup", g.split.Y)
	}
	g.Layout(390, 720)
	want720 := adaptiveMobilePortraitSplitY(720, g)
	if g.split.Y != want720 {
		t.Fatalf("after resize split.Y=%d, want %d", g.split.Y, want720)
	}
}

func TestMobileSplit_RackShowsWholeRows(t *testing.T) {
	for _, h := range []int{640, 720, 844, 960} {
		g := mobileSplitGame(t, 390, h, viewModeRows)
		// Read the size tokens UNDER the mobile profile that mobileSplitGame
		// activates (force-mobile + browser runtime) so they match the values
		// adaptiveMobilePortraitSplitY uses internally. Reading them before the
		// game is created would capture the desktop defaults instead.
		rh := TouchRowHeight()
		headerH := Profile().HeaderMaxH
		if headerH <= 0 {
			headerH = mobileHeaderH
		}
		barH := TouchMinTarget()
		y := adaptiveMobilePortraitSplitY(h, g)
		drumH := h - y
		rackH := drumH - headerH - barH
		if rackH >= rh && rackH%rh != 0 {
			t.Errorf("h=%d: rack %d not whole rows (rh=%d, rem=%d)", h, rackH, rh, rackH%rh)
		}
		if y < h*40/100 {
			t.Errorf("h=%d: snap pushed grid below 40%% (grid=%d)", h, y)
		}
	}
}
