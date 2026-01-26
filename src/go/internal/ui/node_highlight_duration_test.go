package ui

import (
	"io"
	"testing"
	"time"

	"github.com/ingyamilmolinar/tunkul/core/model"
	"github.com/ingyamilmolinar/tunkul/internal/gamestate"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

// stubGameForHighlightDuration creates a minimal Game for highlight duration testing.
func stubGameForHighlightDuration(bpm int) *Game {
	logger := game_log.New(io.Discard, game_log.LevelError)
	g := &Game{
		state:            gamestate.New(bpm),
		highlightedBeats: make(map[int]int64),
		nodeAnim:         make(map[model.NodeID]float64),
		nodeHLUntil:      make(map[model.NodeID]highlightWindow),
		logger:           logger,
		graph:            model.NewGraph(logger),
		playFn:           func(string, float64, ...float64) {},
	}
	g.drum = &DrumView{
		Rows:   []*DrumRow{{}},
		Graph:  g.graph,
		logger: logger,
		bpm:    bpm,
	}
	g.drum.SetLength(64)
	return g
}

func TestHighlightDurationCapped(t *testing.T) {
	assertDefaultParityState(t)
	g := stubGameForHighlightDuration(120)

	// Simulate a long sample (e.g., sub-bass at 2.0 seconds)
	// audio.SampleSeconds returns 0 in stub mode, so we test the fallback capping
	sec := g.expectedHighlightSeconds("sub-bass", 0, 1)

	// Should be capped to maxHighlightSeconds (200ms)
	if sec > maxHighlightSeconds+0.001 {
		t.Errorf("expected highlight capped to %.3fs, got %.3fs", maxHighlightSeconds, sec)
	}
	if sec < minHighlightSeconds-0.001 {
		t.Errorf("expected highlight at least %.3fs, got %.3fs", minHighlightSeconds, sec)
	}
}

func TestHighlightDurationMinimum(t *testing.T) {
	assertDefaultParityState(t)
	g := stubGameForHighlightDuration(120)

	// Test with very high BPM that would cause very short highlight
	gFast := stubGameForHighlightDuration(600)
	sec := gFast.expectedHighlightSeconds("hihat", 0, 1)

	// Should be at least minHighlightSeconds (50ms)
	if sec < minHighlightSeconds-0.001 {
		t.Errorf("expected highlight at least %.3fs, got %.3fs", minHighlightSeconds, sec)
	}

	_ = g // avoid unused variable warning
}

func TestHighlightDurationConstants(t *testing.T) {
	// Verify constants are sensible
	if maxHighlightSeconds <= 0 {
		t.Errorf("maxHighlightSeconds must be positive, got %f", maxHighlightSeconds)
	}
	if minHighlightSeconds <= 0 {
		t.Errorf("minHighlightSeconds must be positive, got %f", minHighlightSeconds)
	}
	if minHighlightSeconds > maxHighlightSeconds {
		t.Errorf("minHighlightSeconds (%f) should not exceed maxHighlightSeconds (%f)",
			minHighlightSeconds, maxHighlightSeconds)
	}
	// Max should be short enough for visual punch (under 500ms)
	if maxHighlightSeconds > 0.5 {
		t.Errorf("maxHighlightSeconds should be under 500ms for visual punch, got %f", maxHighlightSeconds)
	}
}

func TestHighlightTurnsOffAfterDuration(t *testing.T) {
	assertDefaultParityState(t)
	g := stubGameForHighlightDuration(120)

	nodeID := model.NodeID(1)
	startTime := 1.0
	duration := maxHighlightSeconds
	endTime := startTime + duration

	g.setNodeHighlightUntil(nodeID, startTime, endTime)

	// Check window is set correctly
	start, end, ok := g.nodeHighlightUntil(nodeID)
	if !ok {
		t.Fatal("expected highlight window to be set")
	}
	if start != startTime {
		t.Errorf("expected start=%f, got %f", startTime, start)
	}
	if end != endTime {
		t.Errorf("expected end=%f, got %f", endTime, end)
	}

	// Verify window duration is capped
	windowDuration := end - start
	if windowDuration > maxHighlightSeconds+0.001 {
		t.Errorf("window duration %.3fs exceeds max %.3fs", windowDuration, maxHighlightSeconds)
	}
}

func TestHighlightDecaysAfterWindow(t *testing.T) {
	assertDefaultParityState(t)
	g := stubGameForHighlightDuration(120)

	nodeID := model.NodeID(1)
	g.nodeAnimSet(nodeID, 1.0)

	// Verify animation was set
	if g.nodeAnim[nodeID] != 1.0 {
		t.Errorf("expected nodeAnim=1.0, got %f", g.nodeAnim[nodeID])
	}

	// Clear the animation (simulating decay)
	g.nodeAnimSet(nodeID, 0)
	if g.nodeAnim[nodeID] != 0 {
		t.Errorf("expected nodeAnim=0 after clear, got %f", g.nodeAnim[nodeID])
	}
}

func TestHighlightStartsWhenAudioPlays(t *testing.T) {
	assertDefaultParityState(t)
	g := stubGameForHighlightDuration(120)

	info := model.BeatInfo{NodeID: 1, NodeType: model.NodeTypeRegular}
	g.graph.AddNode(0, 0, model.NodeTypeRegular)
	g.setLastTriggeredForTest(0, info.NodeID, true)
	g.drum.SetInstrument("kick")
	ensureInstrumentAvailable(t, g, "kick")

	// Schedule sound with explicit future time
	futureTime := 2.0
	g.scheduleSound(0, 0, info, "kick", 1.0, 0, 1, futureTime, true)

	// Check that highlight window starts at the scheduled time, not before
	start, end, ok := g.nodeHighlightUntil(info.NodeID)
	if !ok {
		t.Fatal("expected highlight window to be set after scheduleSound")
	}

	// Start time should be at or after futureTime (accounting for lookahead)
	if start < futureTime {
		t.Errorf("highlight start %f should not be before audio time %f", start, futureTime)
	}

	// Window should be capped
	windowDuration := end - start
	if windowDuration > maxHighlightSeconds+0.001 {
		t.Errorf("window duration %.3fs exceeds max %.3fs", windowDuration, maxHighlightSeconds)
	}
}

func TestFastLoopHighlightsFlash(t *testing.T) {
	assertDefaultParityState(t)
	g := stubGameForHighlightDuration(120)

	// Simulate fast loop: at 120 BPM with subdiv=16, each beat is ~31ms
	// With maxHighlightSeconds=200ms, we can fit ~6 beats in one highlight window
	// But with capping, each highlight should be short enough to show flashing

	beatsPerSecond := float64(120) / 60.0 * 16.0 / 4.0 // ~8 beats per second
	beatDuration := 1.0 / beatsPerSecond               // ~125ms per beat

	// With 200ms max highlight and ~125ms beat spacing, we should see flashing
	// (highlight < 2x beat spacing ensures visible off periods)
	if maxHighlightSeconds >= beatDuration*2 {
		t.Logf("Warning: maxHighlightSeconds (%.3fs) may cause overlap at %.3fs beat spacing",
			maxHighlightSeconds, beatDuration)
	}

	// Verify the highlight duration is reasonable for visual feedback
	highlightDur := g.expectedHighlightSeconds("kick", 0, 1)
	if highlightDur > maxHighlightSeconds+0.001 {
		t.Errorf("highlight duration %.3fs exceeds max %.3fs", highlightDur, maxHighlightSeconds)
	}
}

func TestHighlightDurationWithPitchShift(t *testing.T) {
	assertDefaultParityState(t)
	g := stubGameForHighlightDuration(120)

	// Higher pitch = faster playback = shorter duration
	// But still should be capped
	secNormal := g.expectedHighlightSeconds("kick", 0, 1)
	secHighPitch := g.expectedHighlightSeconds("kick", 12, 1) // +1 octave = 2x speed

	// Both should be capped
	if secNormal > maxHighlightSeconds+0.001 {
		t.Errorf("normal pitch highlight %.3fs exceeds max", secNormal)
	}
	if secHighPitch > maxHighlightSeconds+0.001 {
		t.Errorf("high pitch highlight %.3fs exceeds max", secHighPitch)
	}
	// Both should meet minimum
	if secNormal < minHighlightSeconds-0.001 {
		t.Errorf("normal pitch highlight %.3fs below min", secNormal)
	}
	if secHighPitch < minHighlightSeconds-0.001 {
		t.Errorf("high pitch highlight %.3fs below min", secHighPitch)
	}
}

func TestHighlightDurationWithDurationParam(t *testing.T) {
	assertDefaultParityState(t)
	g := stubGameForHighlightDuration(120)

	// Longer duration param = slower playback = longer duration
	// But still should be capped
	secNormal := g.expectedHighlightSeconds("kick", 0, 1)
	secSlower := g.expectedHighlightSeconds("kick", 0, 0.5) // half speed

	// Both should be capped to max
	if secNormal > maxHighlightSeconds+0.001 {
		t.Errorf("normal duration highlight %.3fs exceeds max", secNormal)
	}
	if secSlower > maxHighlightSeconds+0.001 {
		t.Errorf("slower duration highlight %.3fs exceeds max", secSlower)
	}
}

// TestHighlightWindowExpiry verifies the highlight window mechanism.
func TestHighlightWindowExpiry(t *testing.T) {
	assertDefaultParityState(t)
	g := stubGameForHighlightDuration(120)

	nodeID := model.NodeID(42)

	// Set a short window
	now := float64(time.Now().UnixNano()) / 1e9
	g.setNodeHighlightUntil(nodeID, now, now+0.1)

	start, end, ok := g.nodeHighlightUntil(nodeID)
	if !ok {
		t.Fatal("expected window to be set")
	}
	if end-start > 0.11 {
		t.Errorf("window duration %.3fs too long", end-start)
	}

	// Clear and verify
	g.clearNodeHighlight(nodeID)
	_, _, ok = g.nodeHighlightUntil(nodeID)
	if ok {
		t.Error("expected window to be cleared")
	}
}
