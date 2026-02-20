//go:build test

package ui

import (
	"math"
	"sync/atomic"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func TestRuntimeAudioLookaheadMaxCap(t *testing.T) {
	assertDefaultParityState(t)
	g := buildTestGame(t)
	g.audioLookaheadSec = 0.04
	g.SetPlayingForTest(true)

	// Simulate high draw times to trigger max extra.
	// snapshot() computes drawAvgMS = drawSumNS / frames / 1e6.
	// To get drawAvgMS=25.0: frames=10, drawSumNS = 25 * 10 * 1e6 = 250_000_000
	g.perf.reset()
	atomic.StoreInt64(&g.perf.frames, 10)
	atomic.StoreInt64(&g.perf.drawSumNS, 250_000_000) // 25ms avg

	got := g.runtimeAudioLookahead()
	// With 40ms base + 30ms extra (DrawAvg > 18) = 70ms, but cap is 60ms
	if got > 0.061 {
		t.Errorf("expected max cap of 60ms, got %.3f", got)
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

// TestExpectedHighlightSecondsUnknownInstrumentFallback verifies the BPM
// fallback path at a normal BPM. Under the test stub, SampleSeconds always
// returns 0, so expectedHighlightSeconds takes the BPM fallback:
// (60/120)*0.25 = 0.125, which falls within [minHighlightSeconds, maxHighlightSeconds].
func TestExpectedHighlightSecondsUnknownInstrumentFallback(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	g.drum.bpm = 120

	sec := g.expectedHighlightSeconds("kick", 0, 1)

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

// TestExpectedHighlightSecondsHighBPMFallback verifies that at very high BPM
// (600), the fallback formula (60/600)*0.25 = 0.025 is below minHighlightSeconds
// and the result is clamped to minHighlightSeconds (0.05).
func TestExpectedHighlightSecondsHighBPMFallback(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	g.drum.bpm = 600

	sec := g.expectedHighlightSeconds("kick", 0, 1)

	raw := (60.0 / 600.0) * 0.25 // 0.025
	if raw >= minHighlightSeconds {
		t.Fatalf("test precondition failed: raw fallback %.3fs should be below min %.3fs", raw, minHighlightSeconds)
	}
	if math.Abs(sec-minHighlightSeconds) > 0.001 {
		t.Fatalf("expected clamped to minHighlightSeconds %.3fs, got %.3fs", minHighlightSeconds, sec)
	}
}

// TestExpectedHighlightSecondsLowBPMFallback verifies that at very low BPM
// (15), the fallback formula (60/15)*0.25 = 1.0 exceeds maxHighlightSeconds
// and the result is clamped to maxHighlightSeconds (0.20).
func TestExpectedHighlightSecondsLowBPMFallback(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	g.drum.bpm = 15

	sec := g.expectedHighlightSeconds("kick", 0, 1)

	raw := (60.0 / 15.0) * 0.25 // 1.0
	if raw <= maxHighlightSeconds {
		t.Fatalf("test precondition failed: raw fallback %.3fs should exceed max %.3fs", raw, maxHighlightSeconds)
	}
	if math.Abs(sec-maxHighlightSeconds) > 0.001 {
		t.Fatalf("expected clamped to maxHighlightSeconds %.3fs, got %.3fs", maxHighlightSeconds, sec)
	}
}

// TestExpectedHighlightSecondsAlwaysPositive verifies that expectedHighlightSeconds
// never returns zero or negative for a range of BPM values.
func TestExpectedHighlightSecondsAlwaysPositive(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	bpms := []int{1, 10, 30, 60, 90, 120, 180, 240, 300, 500, 999}
	for _, bpm := range bpms {
		g.drum.bpm = bpm
		sec := g.expectedHighlightSeconds("kick", 0, 1)
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

// TestScheduleSoundMuteNodeStopsAudio verifies that scheduleSound with a mute
// node type calls audio.Stop for the instrument and returns early without
// enqueuing a sound request.
func TestScheduleSoundMuteNodeStopsAudio(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	// Register and set up the instrument so the row is valid.
	ensureInstrumentAvailable(t, g, "kick")

	var stopped string
	audio.SetStopHook(func(id string) { stopped = id })
	t.Cleanup(func() { audio.SetStopHook(nil) })

	muteNodeID := g.graph.AddNode(0, 0, model.NodeTypeMute)
	info := model.BeatInfo{
		NodeID:   muteNodeID,
		NodeType: model.NodeTypeMute,
		I:        0, J: 0,
	}

	g.scheduleSound(0, 0, info, "kick", 1.0, 0, 1, 0, false)

	if stopped != "kick" {
		t.Fatalf("expected audio.Stop called with %q, got %q", "kick", stopped)
	}
}

func TestAudioChNearFull(t *testing.T) {
	assertDefaultParityState(t)
	g := buildTestGame(t)

	// Fill audioCh to >75% capacity (128 * 0.75 = 96)
	for i := 0; i < 100; i++ {
		select {
		case g.audioCh <- soundReq{id: "fill"}:
		default:
		}
	}

	if !g.audioChNearFull() {
		t.Fatal("expected audioChNearFull() to return true")
	}

	// Drain channel
	for len(g.audioCh) > 0 {
		<-g.audioCh
	}

	if g.audioChNearFull() {
		t.Fatal("expected audioChNearFull() to return false after drain")
	}
}
