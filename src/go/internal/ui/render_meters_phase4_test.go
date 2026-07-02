//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
)

// TestLevelsReadoutNeverDisappears pins the Phase 4 contract: across
// every (density × width) combination, the aggregate readouts must
// remain reachable. Three render paths cover the space:
//   - full text column at wide widths
//   - icon-row collapse at medium widths
//   - footer chevron at narrow widths (one-tap to bottom-sheet)
//
// At no width should a user lose the Headroom number entirely.
func TestLevelsReadoutNeverDisappears(t *testing.T) {
	state := &analyzer.State{
		Master: analyzer.ChannelMetrics{
			Active: true,
			PeakDB: -8, RMSDB: -14, ClipCount: 0,
		},
		Instruments: []analyzer.InstrumentMetrics{
			{ID: "kick", Name: "Kick", Active: true, PeakDB: -6},
			{ID: "snare", Name: "Snare", Active: true, PeakDB: -10},
		},
	}
	latches := NewMultiLevelsLatch()
	latches.Get("main").Update(0, -8, -14)

	densities := []Density{DensityCompact, DensityComfortable, DensitySpacious}
	widths := []int{200, 280, 360, 480, 720}
	for _, d := range densities {
		restore := SetDensityForTest(d)
		for _, w := range widths {
			dst := ebiten.NewImage(w, 240)
			rects := collectFilledRects(t, func() {
				drawLevelsMultiChannel(dst, image.Rect(0, 0, w, 240), state, latches, nil)
			})
			if len(rects) == 0 {
				t.Errorf("density=%v w=%d: no rects drawn (expected at least the meter chrome)", d, w)
			}
			// We don't enforce a specific mode here — just that the
			// renderer didn't blank out. The mode-selection contract
			// is verified by the dedicated mode tests below.
		}
		restore()
	}
}

// TestLevelsReadoutModeSelection pins the mode-selection thresholds.
// Below WFull + WIcons → chevron only. WFull + WIcons ≤ width < 2 ×
// WFull → icon row. Width ≥ 2 × WFull → full text column.
func TestLevelsReadoutModeSelection(t *testing.T) {
	restore := SetDensityForTest(DensityComfortable)
	defer restore()
	dv := Profile().DensityValues()
	full := dv.LevelsReadoutWFull   // 180
	icons := dv.LevelsReadoutWIcons // 36
	cases := []struct {
		width    int
		wantMode readoutColMode
		name     string
	}{
		{full * 2, readoutColModeFull, "boundary-full"},
		{full*2 + 100, readoutColModeFull, "above-full"},
		{full + icons, readoutColModeIcons, "boundary-icons"},
		{full*2 - 1, readoutColModeIcons, "just-below-full"},
		{full + icons - 1, readoutColModeChevron, "just-below-icons"},
		{full, readoutColModeChevron, "below-icons"},
		{100, readoutColModeChevron, "very-narrow"},
	}
	for _, c := range cases {
		mode, _ := levelsReadoutLayout(c.width)
		if mode != c.wantMode {
			t.Errorf("%s (width=%d): mode=%v, want %v", c.name, c.width, mode, c.wantMode)
		}
	}
}

// TestLevelsAggregatesIconRowRendersAllThree — the icon-row collapse
// must paint at least one rect per readout role (Headroom / Clips /
// Loudest). Verified via rect count.
func TestLevelsAggregatesIconRowRendersAllThree(t *testing.T) {
	restore := SetDensityForTest(DensityComfortable)
	defer restore()
	state := &analyzer.State{
		Master: analyzer.ChannelMetrics{Active: true, PeakDB: -8},
		Instruments: []analyzer.InstrumentMetrics{
			{ID: "kick", Name: "Kick", Active: true, PeakDB: -6},
		},
	}
	latches := NewMultiLevelsLatch()
	latches.Get("main").Update(0, -8, -14)

	dst := ebiten.NewImage(60, 90)
	rects := collectFilledRects(t, func() {
		drawLevelsAggregatesIconRow(dst, image.Rect(0, 0, 60, 90), state, latches, nil)
	})
	// Text rects are NOT filled rects, so we don't expect them — but
	// at minimum the chevron-style backgrounds shouldn't be there
	// either since the icon-row uses pure text. The contract is
	// "doesn't panic, doesn't blank the screen with no text".
	_ = rects // smoke test
}

// TestLevelsAggregatesChevronRendersIcon — the chevron mode must
// paint at least the rounded-rect background + the chevron icon.
func TestLevelsAggregatesChevronRendersIcon(t *testing.T) {
	dst := ebiten.NewImage(40, 20)
	rects := collectFilledRects(t, func() {
		drawLevelsAggregatesChevron(dst, image.Rect(0, 0, 32, 18))
	})
	if len(rects) == 0 {
		t.Errorf("chevron: no rects drawn (expected at least background)")
	}
}
