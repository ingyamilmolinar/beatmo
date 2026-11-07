//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// Recomputed like the implementation, so the test pins the formula, not a pixel value.
func wantReadoutContentW(t *testing.T) int {
	t.Helper()
	w := 0
	type cand struct {
		s    string
		role TextRole
	}
	for _, c := range []cand{
		{i18n.T(i18n.KeyCapHeadroom), RoleCaption},
		{i18n.T(i18n.KeyLevelsClips) + " (10s)", RoleCaption},
		{"LUFS-S", RoleCaption},
		{i18n.T(i18n.KeyCapLoudest), RoleCaption},
		{"+99 dB", RolePanelTitle},
		{"CLIP!", RolePanelTitle},
	} {
		if tw := StyledTextWidth(c.s, c.role); tw > w {
			w = tw
		}
	}
	w += levelsReadoutFullPadX
	if w < levelsReadoutFullMinW {
		w = levelsReadoutFullMinW
	}
	return w
}

func TestLevelsReadoutFullW_ContentFitAndSmallerThanToken(t *testing.T) {
	restore := SetDensityForTest(DensityComfortable)
	defer restore()
	got := levelsReadoutFullW()
	want := wantReadoutContentW(t)
	if want <= Profile().DensityValues().LevelsReadoutWFull && got != want {
		t.Fatalf("levelsReadoutFullW(): got %d, want content-fit %d", got, want)
	}
	if got >= Profile().DensityValues().LevelsReadoutWFull {
		t.Fatalf("content-fit summary (%d) must be SMALLER than the LevelsReadoutWFull budget (%d) — the whole point is to reclaim width for bars",
			got, Profile().DensityValues().LevelsReadoutWFull)
	}
}

// The summary takes well under half the panel.
func TestLevelsReadoutLayout_FullModeBarsGetMajority(t *testing.T) {
	restore := SetDensityForTest(DensityComfortable)
	defer restore()
	mode, rw := levelsReadoutLayout(800)
	if mode != readoutColModeFull {
		t.Fatalf("wide desktop panel should be full mode; got %v", mode)
	}
	if rw != levelsReadoutFullW() {
		t.Fatalf("full-mode readoutW should be content-fit %d; got %d", levelsReadoutFullW(), rw)
	}
	bars := 800 - rw
	if bars <= rw {
		t.Fatalf("bars region (%d) must exceed summary (%d) — bars take the most space", bars, rw)
	}
}

// Constant across widths, so the bars take every extra pixel.
func TestLevelsReadoutLayout_SummaryWidthConstant_BarsGrow(t *testing.T) {
	restore := SetDensityForTest(DensityComfortable)
	defer restore()
	_, rwSmall := levelsReadoutLayout(600)
	_, rwLarge := levelsReadoutLayout(1200)
	if rwSmall != rwLarge {
		t.Fatalf("summary width must be constant across panel widths; 600→%d, 1200→%d", rwSmall, rwLarge)
	}
	if (1200 - rwLarge) <= (600 - rwSmall) {
		t.Fatalf("bars region must grow with panel width while summary stays fixed")
	}
}

// A phone-width Spacious panel stays in icon mode.
func TestLevelsReadoutLayout_MobileStaysIcons(t *testing.T) {
	restore := SetDensityForTest(DensitySpacious)
	defer restore()
	mode, rw := levelsReadoutLayout(390)
	if mode != readoutColModeIcons {
		t.Fatalf("mobile panel must stay in icons mode; got %v", mode)
	}
	if rw != Profile().DensityValues().LevelsReadoutWIcons {
		t.Fatalf("icons-mode width should be the icons token %d; got %d", Profile().DensityValues().LevelsReadoutWIcons, rw)
	}
}
