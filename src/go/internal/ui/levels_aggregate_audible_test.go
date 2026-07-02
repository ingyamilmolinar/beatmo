//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
)

func aggAudibleState() *analyzer.State {
	return &analyzer.State{
		Instruments: []analyzer.InstrumentMetrics{
			{ID: "kick", Name: "kick", PeakDB: -6, ClipCount: 5, Active: true},
			{ID: "snare", Name: "snare", PeakDB: -8, ClipCount: 1, Active: true},
		},
		Master: analyzer.ChannelMetrics{ID: "main", Name: "Master", PeakDB: -3, ClipCount: 0},
	}
}

// With only snare audible, Loudest must be snare (not the louder-but-hidden
// kick) and the clip total must exclude kick's clips.
func TestLevelsAggregates_FilteredToAudible(t *testing.T) {
	vis := map[string]bool{"snare": true}
	agg := levelsAggregatesValues(aggAudibleState(), nil, vis)
	if agg.LoudestName != "snare" {
		t.Fatalf("loudest should be snare (kick hidden); got %q", agg.LoudestName)
	}
	if agg.ClipsTotal != 1 {
		t.Fatalf("clips total should exclude hidden kick(5), keep snare(1)+master(0); got %d", agg.ClipsTotal)
	}
}

// nil visibleIDs preserves the legacy global aggregate behavior.
func TestLevelsAggregates_NilShowsAll(t *testing.T) {
	agg := levelsAggregatesValues(aggAudibleState(), nil, nil)
	if agg.LoudestName != "kick" {
		t.Fatalf("nil → loudest should be kick; got %q", agg.LoudestName)
	}
	if agg.ClipsTotal != 6 {
		t.Fatalf("nil → clips total kick(5)+snare(1)+master(0)=6; got %d", agg.ClipsTotal)
	}
}
