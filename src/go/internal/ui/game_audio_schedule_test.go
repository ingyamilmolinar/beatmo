//go:build test

package ui

import (
	"math"
	"sync/atomic"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func TestRuntimeAudioLookaheadMaxCap(t *testing.T) {
	assertDefaultParityState(t)
	g := buildTestGame(t)
	// High enough that base + the 30 ms extra crosses the 150 ms ceiling: the cap must win.
	g.audioLookaheadSec = 0.14
	g.SetPlayingForTest(true)

	// drawAvgMS = drawSumNS / frames / 1e6, so 25 ms at 10 frames.
	g.perf.reset()
	atomic.StoreInt64(&g.perf.frames, 10)
	atomic.StoreInt64(&g.perf.drawSumNS, 250_000_000) // 25ms avg

	got := g.runtimeAudioLookahead()
	// 140ms base + 30ms extra (DrawAvg > 18) = 170ms, clamped to the 150ms cap.
	if got > 0.151 {
		t.Errorf("expected max cap of 150ms, got %.3f", got)
	}
	if math.Abs(got-0.15) > 1e-9 {
		t.Errorf("expected cap to clamp to exactly 0.15, got %.3f", got)
	}
}

func TestRuntimeAudioLookaheadZeroRespected(t *testing.T) {
	assertDefaultParityState(t)
	g := buildTestGame(t)
	g.audioLookaheadSec = 0

	got := g.runtimeAudioLookahead()
	if got != 0 {
		t.Errorf("expected 0 when base is 0, got %f", got)
	}
}

func TestRuntimeAudioLookaheadNotPlayingCap(t *testing.T) {
	assertDefaultParityState(t)
	g := buildTestGame(t)
	g.audioLookaheadSec = 0.04

	// Not playing: should cap at 60ms (new cap)
	got := g.runtimeAudioLookahead()
	if got > 0.061 {
		t.Errorf("expected not-playing cap of 60ms, got %.3f", got)
	}
}

// The test stub's SampleSeconds is 0, so the BPM fallback: (60/120)·0.25 = 0.125.
func TestExpectedHighlightSecondsUnknownInstrumentFallback(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	g.drum.bpm = 120

	sec := g.expectedHighlightSeconds(roster.Kick, 0, 1)

	expected := (60.0 / 120.0) * 0.25 // 0.125
	if math.Abs(sec-expected) > 0.001 {
		t.Fatalf("at 120 BPM expected %.3fs, got %.3fs", expected, sec)
	}
	if sec < minHighlightSeconds {
		t.Fatalf("result %.3fs is below minHighlightSeconds %.3fs", sec, minHighlightSeconds)
	}
	if sec > maxHighlightSeconds {
		t.Fatalf("result %.3fs exceeds maxHighlightSeconds %.3fs", sec, maxHighlightSeconds)
	}
}

// (60/600)·0.25 = 0.025, clamped up to 0.05.
func TestExpectedHighlightSecondsHighBPMFallback(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	g.drum.bpm = 600

	sec := g.expectedHighlightSeconds(roster.Kick, 0, 1)

	raw := (60.0 / 600.0) * 0.25 // 0.025
	if raw >= minHighlightSeconds {
		t.Fatalf("test precondition failed: raw fallback %.3fs should be below min %.3fs", raw, minHighlightSeconds)
	}
	if math.Abs(sec-minHighlightSeconds) > 0.001 {
		t.Fatalf("expected clamped to minHighlightSeconds %.3fs, got %.3fs", minHighlightSeconds, sec)
	}
}

// (60/15)·0.25 = 1.0, clamped down to 0.20.
func TestExpectedHighlightSecondsLowBPMFallback(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	g.drum.bpm = 15

	sec := g.expectedHighlightSeconds(roster.Kick, 0, 1)

	raw := (60.0 / 15.0) * 0.25 // 1.0
	if raw <= maxHighlightSeconds {
		t.Fatalf("test precondition failed: raw fallback %.3fs should exceed max %.3fs", raw, maxHighlightSeconds)
	}
	if math.Abs(sec-maxHighlightSeconds) > 0.001 {
		t.Fatalf("expected clamped to maxHighlightSeconds %.3fs, got %.3fs", maxHighlightSeconds, sec)
	}
}

func TestExpectedHighlightSecondsAlwaysPositive(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	bpms := []int{1, 10, 30, 60, 90, 120, 180, 240, 300, 500, 999}
	for _, bpm := range bpms {
		g.drum.bpm = bpm
		sec := g.expectedHighlightSeconds(roster.Kick, 0, 1)
		if sec <= 0 {
			t.Errorf("BPM=%d: expectedHighlightSeconds returned non-positive %.6f", bpm, sec)
		}
		if sec < minHighlightSeconds-0.001 {
			t.Errorf("BPM=%d: result %.6f below min %.3f", bpm, sec, minHighlightSeconds)
		}
		if sec > maxHighlightSeconds+0.001 {
			t.Errorf("BPM=%d: result %.6f above max %.3f", bpm, sec, maxHighlightSeconds)
		}
	}
}

// A mute node stops the instrument and enqueues nothing.
func TestScheduleSoundMuteNodeStopsAudio(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	// Register and set up the instrument so the row is valid.
	ensureInstrumentAvailable(t, g, roster.Kick)

	var stopped instrument.ID
	audio.SetStopHook(func(id instrument.ID) { stopped = id })
	t.Cleanup(func() { audio.SetStopHook(nil) })

	muteNodeID := g.graph.AddNode(0, 0, model.NodeTypeMute)
	info := model.BeatInfo{
		NodeID:   muteNodeID,
		NodeType: model.NodeTypeMute,
		I:        0, J: 0,
	}

	g.scheduleSound(0, 0, info, roster.Kick, 1.0, 0, 1, 0, false)

	if stopped != roster.Kick {
		t.Fatalf("expected audio.Stop called with %q, got %q", "kick", stopped)
	}
}

// Every change shifts all later notes, so an oscillating cushion is audible; a new generation starts from base.
func TestRuntimeAudioLookaheadNeverShrinksMidPlay(t *testing.T) {
	assertDefaultParityState(t)
	g := buildTestGame(t)
	g.audioLookaheadSec = 0.06
	g.SetPlayingForTest(true)

	setDrawAvg := func(ms int64) {
		g.perf.reset()
		atomic.StoreInt64(&g.perf.frames, 10)
		atomic.StoreInt64(&g.perf.drawSumNS, ms*10*1_000_000)
	}
	setDrawAvg(25) // > 18ms → +30ms
	if got := g.runtimeAudioLookahead(); math.Abs(got-0.09) > 1e-9 {
		t.Fatalf("under pressure: want 0.090, got %.3f", got)
	}
	setDrawAvg(1) // calm frame: raw value would drop back to 0.06
	if got := g.runtimeAudioLookahead(); math.Abs(got-0.09) > 1e-9 {
		t.Fatalf("calm frame in the same session shrank the cushion to %.3f; want 0.090", got)
	}
	g.audioGen.Add(1) // Stop/Pause/seek → new transport generation
	if got := g.runtimeAudioLookahead(); math.Abs(got-0.06) > 1e-9 {
		t.Fatalf("new transport generation: want base 0.060, got %.3f", got)
	}
}
