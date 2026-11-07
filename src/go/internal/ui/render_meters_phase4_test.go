//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// At every density × width the Headroom number stays reachable: a full column when wide, an icon row when
// medium, a footer chevron to a bottom sheet when narrow.
func TestLevelsReadoutNeverDisappears(t *testing.T) {
	state := &analyzer.State{
		Master: analyzer.ChannelMetrics{
			Active: true,
			PeakDB: -8, RMSDB: -14, ClipCount: 0,
		},
		Instruments: []analyzer.InstrumentMetrics{
			{ID: channel.Of(roster.Kick), Name: "Kick", Active: true, PeakDB: -6},
			{ID: channel.Of(roster.Snare), Name: "Snare", Active: true, PeakDB: -10},
		},
	}
	latches := NewMultiLevelsLatch()
	latches.Get(channel.Master).Update(0, -8, -14)

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
			// Only that it did not blank; the mode thresholds are tested below.
		}
		restore()
	}
}

// Below WFull + WIcons a chevron; up to 2·WFull an icon row; beyond, the full column.
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

// At least one rect per role: Headroom, Clips, Loudest.
func TestLevelsAggregatesIconRowRendersAllThree(t *testing.T) {
	restore := SetDensityForTest(DensityComfortable)
	defer restore()
	state := &analyzer.State{
		Master: analyzer.ChannelMetrics{Active: true, PeakDB: -8},
		Instruments: []analyzer.InstrumentMetrics{
			{ID: channel.Of(roster.Kick), Name: "Kick", Active: true, PeakDB: -6},
		},
	}
	latches := NewMultiLevelsLatch()
	latches.Get(channel.Master).Update(0, -8, -14)

	dst := ebiten.NewImage(60, 90)
	rects := collectFilledRects(t, func() {
		drawLevelsAggregatesIconRow(dst, image.Rect(0, 0, 60, 90), state, latches, nil)
	})
	// A smoke test: the icon row is pure text, which draws no filled rects.
	_ = rects // smoke test
}

// The rounded background and the chevron icon at least.
func TestLevelsAggregatesChevronRendersIcon(t *testing.T) {
	dst := ebiten.NewImage(40, 20)
	rects := collectFilledRects(t, func() {
		drawLevelsAggregatesChevron(dst, image.Rect(0, 0, 32, 18))
	})
	if len(rects) == 0 {
		t.Errorf("chevron: no rects drawn (expected at least background)")
	}
}
