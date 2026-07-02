//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// wantReadoutContentW recomputes the content-fit width the SAME way the
// implementation must, so the test pins the FORMULA (labels + widest big
// number + padding), not a magic px value.
func wantReadoutContentW(t *testing.T) int {
	t.Helper()
	capScale := FontSizeCaption / FontSizeBody
	headlineScale := FontSizeHeading / FontSizeBody
	w := 0
	type cand struct {
		s  string
		sc float64
	}
	for _, c := range []cand{
		{i18n.T(i18n.KeyCapHeadroom), capScale},
		{i18n.T(i18n.KeyLevelsClips) + " (10s)", capScale},
		{"LUFS-S", capScale},
		{i18n.T(i18n.KeyCapLoudest), capScale},
		{"+99 dB", headlineScale},
		{"CLIP!", headlineScale},
	} {
		if tw := int(float64(TextWidth(c.s)) * c.sc); tw > w {
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

// Full mode returns the content-fit width, and the summary takes well under
// half the panel so the bars get the majority.
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

// "Do not change the summary width": the summary width is identical across
// different full-mode panel widths, so the bars absorb every extra pixel.
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

// Regression guard: mobile (Spacious) at a phone-width panel must STAY in the
// icon-row mode (the 34px column from the mobile bar-room task), NOT flip to
// the full column.
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
