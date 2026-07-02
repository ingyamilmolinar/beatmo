//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
)

func filterState() *analyzer.State {
	return &analyzer.State{
		Instruments: []analyzer.InstrumentMetrics{
			{ID: "kick", Name: "kick", PeakDB: -6, RMSDB: -18, Active: true},
			{ID: "snare", Name: "snare", PeakDB: -8, RMSDB: -20, Active: true},
			{ID: "hat", Name: "hat", PeakDB: -10, RMSDB: -22, Active: true},
		},
		Master: analyzer.ChannelMetrics{ID: "main", Name: "Master", PeakDB: -3, RMSDB: -14, Active: true},
	}
}

// collectDrawnTexts runs fn while intercepting DrawTextColorAtScale and
// returns the set of strings that were drawn.
func collectDrawnTexts(t *testing.T, fn func()) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	orig := drawTextColorAtScaleHook
	drawTextColorAtScaleHook = func(s string) { out[s] = true }
	defer func() { drawTextColorAtScaleHook = orig }()
	fn()
	return out
}

// With a visibleIDs set of one instrument, only that instrument's strip text
// plus the Master strip text should be drawn; hidden instrument names must not.
func TestLevelsVisibleFilter_OnlyVisibleStrips(t *testing.T) {
	rect := image.Rect(0, 0, 800, 120)
	dst := ebiten.NewImage(rect.Dx(), rect.Dy())
	visible := map[string]bool{"hat": true}

	labels := collectDrawnTexts(t, func() {
		drawLevelsMultiChannel(dst, rect, filterState(), NewMultiLevelsLatch(), visible)
	})

	if !labels["hat"] {
		t.Fatalf("expected 'hat' strip label drawn; texts=%v", labels)
	}
	if labels["kick"] || labels["snare"] {
		t.Fatalf("hidden instrument labels must not be drawn; texts=%v", labels)
	}
}

// nil visibleIDs preserves the legacy "show everything" behavior.
func TestLevelsVisibleFilter_NilShowsAll(t *testing.T) {
	rect := image.Rect(0, 0, 800, 120)
	dst := ebiten.NewImage(rect.Dx(), rect.Dy())

	labels := collectDrawnTexts(t, func() {
		drawLevelsMultiChannel(dst, rect, filterState(), NewMultiLevelsLatch(), nil)
	})
	for _, id := range []string{"kick", "snare", "hat"} {
		if !labels[id] {
			t.Fatalf("nil visibleIDs should draw all strips; missing %q in %v", id, labels)
		}
	}
}

// Driving the real EQ panel Lvl Draw with one soloed row should restrict the
// Levels strips to that instrument + Master (kick is muted-by-solo; hat has
// no row, so neither should appear as a strip label).
func TestEQPanelLevels_SoloRestrictsStrips(t *testing.T) {
	withDefaultAudio(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	z := g.drum.eqPanelZone
	if z == nil {
		buildSoakScene(t, g, 3, 4)
		z = g.drum.eqPanelZone
	}
	if z == nil {
		t.Fatal("eqPanelZone nil; harness assumption broken")
	}

	rows := []*DrumRow{
		{Instrument: "kick"},
		{Instrument: "snare", Solo: true},
	}
	z.callbacks.ActiveRows = func() []*DrumRow { return rows }
	st := filterState() // kick(-6) loudest, snare(-8), hat(-10) + master
	z.callbacks.AnalyzerState = func() *analyzer.State { return st }
	z.callbacks.AnalyzerMetricsOnly = func() *analyzer.State { return st }

	z.tabState.SetActiveTab(TabMeters)
	scratch := ebiten.NewImage(1280, 720)

	labels := collectDrawnTexts(t, func() { z.Draw(scratch) })
	if !labels["snare"] {
		t.Fatalf("solo snare should show the snare strip label; texts=%v", labels)
	}
	if labels["kick"] {
		t.Fatalf("kick is muted-by-solo and must not appear as a strip label; texts=%v", labels)
	}
}
