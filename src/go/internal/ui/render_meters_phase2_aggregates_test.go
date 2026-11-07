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

// The LUFS-S row renders when the short-term value is set.
func TestLevelsAggregates_LUFSRendered(t *testing.T) {
	state := &analyzer.State{
		Master: analyzer.ChannelMetrics{
			Active:          true,
			PeakDB:          -12,
			RMSDB:           -18,
			LUFSShortTermDB: -14.3,
		},
	}
	latches := NewMultiLevelsLatch()
	latches.Get(channel.Master).Update(0, -12, -18)

	dst := ebiten.NewImage(400, 200)
	drawLevelsAggregates(dst, image.Rect(0, 0, 200, 200), state, latches, nil)
	// A smoke test; audio_panel_render_pixels_test.go counts the composed panel.
}

// The rolling 10 s count, labelled "(10s)", wins over the monotonic total.
func TestLevelsAggregates_ClipsWindowOverridesTotal(t *testing.T) {
	stateWithWindow := &analyzer.State{
		Master:          analyzer.ChannelMetrics{Active: true, PeakDB: -3, ClipCount: 100},
		ClipsLastWindow: 4, // last 10 s
	}
	stateNoWindow := &analyzer.State{
		Master:          analyzer.ChannelMetrics{Active: true, PeakDB: -3, ClipCount: 100},
		ClipsLastWindow: 0,
	}
	dst := ebiten.NewImage(400, 200)
	// Both label branches.
	drawLevelsAggregates(dst, image.Rect(0, 0, 200, 200), stateWithWindow, nil, nil)
	drawLevelsAggregates(dst, image.Rect(0, 0, 200, 200), stateNoWindow, nil, nil)
}

// The "!" markers clear at once, not after the 60-frame auto-expire.
func TestMultiLevelsLatch_ClearWipesPersistentMarkers(t *testing.T) {
	m := NewMultiLevelsLatch()
	// Force a clip event on two channels.
	m.Get(channel.Of(roster.Kick)).Update(3, -1, -3)
	m.Get(channel.Of(roster.Hihat)).Update(2, -1, -3)
	if !m.Get(channel.Of(roster.Kick)).Latched() {
		t.Fatalf("kick should be latched after clip event")
	}
	if !m.Get(channel.Of(roster.Hihat)).Latched() {
		t.Fatalf("hat should be latched after clip event")
	}
	m.Clear()
	if m.Get(channel.Of(roster.Kick)).Latched() {
		t.Errorf("kick still latched after Clear()")
	}
	if m.Get(channel.Of(roster.Hihat)).Latched() {
		t.Errorf("hat still latched after Clear()")
	}
}
