//go:build test

package ui

import (
	"image"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// The in-panel header is gone at every density on mobile; its actions live on the sticky bar.
func TestSynthMobileHeaderRelocatedAtAllDensities(t *testing.T) {
	for _, d := range []Density{DensityCompact, DensityComfortable, DensitySpacious} {
		restoreDensity := SetDensityForTest(d)
		forceSmallScreenForTest = true
		t.Cleanup(func() { forceSmallScreenForTest = false })

		dv := &DrumView{}
		btnH := Profile().DensityValues().SynthHeaderButtonH
		headerH := 2*SpaceSM + TextHeight() + SpaceSM + btnH
		dv.layoutSynthHeader(image.Rect(0, 0, 360, 200), headerH, roster.KickSnap, true)
		h := dv.instEditorHeader

		for name, r := range map[string]image.Rectangle{
			"preview": h.previewRect, "save-as": h.saveAsRect,
		} {
			if !r.Empty() {
				t.Errorf("density=%v: in-panel %q rect %v is non-empty (must be relocated off-panel on mobile)", d, name, r)
			}
		}
		restoreDensity()
	}
}

// On mobile the three modes are an always-visible contiguous segmented row: touch-min, no gaps, outside the
// trace, each with a hit area.
func TestChainModeSegmentedOnMobile(t *testing.T) {
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })
	restoreDensity := SetDensityForTest(DensitySpacious)
	defer restoreDensity()

	if !Profile().ChainModeSegmented {
		t.Fatal("Profile().ChainModeSegmented=false on mobile (expected true)")
	}

	z := NewChainPanelZone(ChainCallbacks{})
	z.Layout(image.Rect(0, 0, 360, 240))

	ov, sp, df := z.overlayBtn.Rect(), z.splitBtn.Rect(), z.diffBtn.Rect()
	minTarget := Profile().MinTarget
	for name, r := range map[string]image.Rectangle{"overlay": ov, "split": sp, "diff": df} {
		if r.Empty() {
			t.Errorf("%s pill rect empty on mobile (must be visible segmented)", name)
		}
		if r.Dy() < minTarget {
			t.Errorf("%s pill height %d < MinTarget %d (not touch-min)", name, r.Dy(), minTarget)
		}
	}

	// Contiguous segmented: each pill abuts the next (no gap).
	if ov.Max.X != sp.Min.X {
		t.Errorf("overlay/split not contiguous: %d != %d", ov.Max.X, sp.Min.X)
	}
	if sp.Max.X != df.Min.X {
		t.Errorf("split/diff not contiguous: %d != %d", sp.Max.X, df.Min.X)
	}

	// The segmented row must not overlap the trace content area.
	cr := z.contentRect()
	if ov.Max.Y > cr.Min.Y {
		t.Errorf("segmented row (bottom %d) overlaps trace area (top %d)", ov.Max.Y, cr.Min.Y)
	}

	// Each mode pill keeps its hit area.
	tags := map[string]bool{"scope-overlay-btn": false, "scope-split-btn": false, "scope-diff-btn": false}
	for _, h := range z.HitAreas() {
		if _, ok := tags[h.Tag]; ok {
			tags[h.Tag] = true
		}
	}
	for tag, found := range tags {
		if !found {
			t.Errorf("missing mode hit area %q on mobile segmented control", tag)
		}
	}

	// The retired dropdown chip must be gone.
	for _, h := range z.HitAreas() {
		if h.Tag == "scope-mode-dropdown-btn" {
			t.Error("retired scope-mode-dropdown-btn hit area still present")
		}
	}
}

// A long press on the icon row opens a tooltip with the formatted value ("Headroom: 7.3 dB").
func TestLevelsReadoutTooltipSurfacesValues(t *testing.T) {
	z, _ := newTestEQPanelZone(nil)
	tree := registerEQZone(z, image.Rect(0, 0, 360, 200))
	z.tabState.SetActiveTab(TabMeters)

	// Known values, so the strings are deterministic.
	state := &analyzer.State{
		Master: analyzer.ChannelMetrics{PeakDB: -7.3, ClipCount: 12},
		Instruments: []analyzer.InstrumentMetrics{
			{ID: channel.Of(roster.Kick), Name: "Kick", PeakDB: -3},
			{ID: channel.Of(roster.Snare), Name: "Snare", PeakDB: -10},
		},
	}
	// Set directly: the icon-row threshold derives from density, so laying out a real panel at that exact width is
	// fragile.
	r := image.Rect(0, 0, 40, 120)
	z.levelsHeadroomRect, z.levelsClipsRect, z.levelsLoudestRect = levelsAggregateIconRects(r)
	z.levelsAggSnapshot = levelsAggregatesValues(state, nil, nil)

	// Long-press over the Headroom icon → tooltip with "Headroom".
	headroomCenter := image.Pt(
		z.levelsHeadroomRect.Min.X+z.levelsHeadroomRect.Dx()/2,
		z.levelsHeadroomRect.Min.Y+z.levelsHeadroomRect.Dy()/2,
	)
	if !z.HandleLevelsAggregateLongPress(headroomCenter.X, headroomCenter.Y) {
		t.Fatal("HandleLevelsAggregateLongPress(headroom point) returned false")
	}
	if !tree.Portal().Has("levels-agg-tt") {
		t.Fatal("levels-agg-tt portal entry missing after headroom long-press")
	}
	text := levelsAggregateTooltipText("headroom", z.levelsAggSnapshot)
	if !strings.Contains(text, "Headroom") {
		t.Errorf("tooltip text %q does not mention Headroom", text)
	}
	if !strings.Contains(text, z.levelsAggSnapshot.HeadroomTxt) {
		t.Errorf("tooltip text %q missing headroom value %q",
			text, z.levelsAggSnapshot.HeadroomTxt)
	}

	// Loudest slot should surface the loudest channel name.
	loudCenter := image.Pt(
		z.levelsLoudestRect.Min.X+z.levelsLoudestRect.Dx()/2,
		z.levelsLoudestRect.Min.Y+z.levelsLoudestRect.Dy()/2,
	)
	if !z.HandleLevelsAggregateLongPress(loudCenter.X, loudCenter.Y) {
		t.Fatal("HandleLevelsAggregateLongPress(loudest point) returned false")
	}
	loudText := levelsAggregateTooltipText("loudest", z.levelsAggSnapshot)
	if !strings.Contains(loudText, "Kick") {
		t.Errorf("loudest tooltip text %q should name the loudest channel (Kick)", loudText)
	}

	// Outside any icon: returns false, no new tooltip churn.
	if z.HandleLevelsAggregateLongPress(-1, -1) {
		t.Error("HandleLevelsAggregateLongPress(off-icon) returned true")
	}
}

// A tap-to-edit target on mobile.
func TestDBInputMeetsTouchMinAtSpacious(t *testing.T) {
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })
	restoreDensity := SetDensityForTest(DensitySpacious)
	defer restoreDensity()

	z, _ := newTestEQPanelZone(nil)
	registerEQZone(z, image.Rect(0, 0, 600, 240))
	z.tabState.SetActiveTab(TabEQ)
	z.Layout(image.Rect(0, 0, 600, 240))

	minTarget := Profile().MinTarget
	for i := range z.dbReadoutRects {
		r := z.dbReadoutRects[i]
		if r.Empty() {
			continue
		}
		h := r.Dy()
		if h < minTarget {
			t.Errorf("dB readout %d height=%d < MinTarget=%d", i, h, minTarget)
		}
	}
}

// BtnHeightMD² at Comfortable, MinTarget² at Spacious: missing a band bypass to a sub-thumb target is a real bug.
func TestEqMuteButtonHitAreaAtComfortablePlus(t *testing.T) {
	for _, d := range []Density{DensityComfortable, DensitySpacious} {
		restoreDensity := SetDensityForTest(d)
		z, _ := newTestEQPanelZone(nil)
		registerEQZone(z, image.Rect(0, 0, 600, 240))
		z.tabState.SetActiveTab(TabEQ)
		z.Layout(image.Rect(0, 0, 600, 240))

		minTarget := Profile().MinTarget
		floor := BtnHeightMD
		if d == DensitySpacious {
			floor = minTarget
		}
		for i, b := range z.eqMuteBtns {
			if b == nil {
				continue
			}
			h := b.Rect().Dy()
			if h < floor {
				t.Errorf("density=%v: mute button %d height=%d < floor=%d",
					d, i, h, floor)
			}
		}
		restoreDensity()
	}
}

func containsString(xs []string, want string) bool {
	for _, s := range xs {
		if s == want {
			return true
		}
	}
	return false
}
