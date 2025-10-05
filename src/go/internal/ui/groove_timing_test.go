package ui

import (
	"math"
	"os"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

// Helper: wait for BPM apply
func waitApply(g *Game, d time.Duration) {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		_ = g.Update()
		time.Sleep(5 * time.Millisecond)
	}
}

// Build a simple 1-beat edge with two nodes: 0 -> 32.
func buildEdge(g *Game) (a, b *uiNode) {
	g.pendingStartRow = 0
	a = g.tryAddNode(0, 0, model.NodeTypeRegular)
	b = g.tryAddNode(32, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.pendingStartRow = -1
	g.updateBeatInfos()
	return
}

// Enable with BPM_TIMING_TEST=1
func TestGrooveDelaySchedulesWithinOneSubdivision(t *testing.T) {
	if os.Getenv("BPM_TIMING_TEST") != "1" {
		t.Skip("set BPM_TIMING_TEST=1")
	}
	logger := game_log.New(os.Stdout, game_log.LevelError)
	g := New(logger)
	g.Layout(800, 600)
	// Freeze input
	restore := SetInputForTest(func() (int, int) { return 0, 0 }, func(ebiten.MouseButton) bool { return false }, func(ebiten.Key) bool { return false }, func() []rune { return nil }, func() (float64, float64) { return 0, 0 }, func() (int, int) { return 800, 600 })
	defer restore()

	a, _ := buildEdge(g)
	// Set node groove: Delay 100%
	if n, ok := g.graph.GetNodeByID(a.ID); ok {
		p := n.Params
		p.GrooveKind = "delay"
		p.GroovePct = 1.0
		g.graph.SetNodeParams(a.ID, p)
	}
	// Capture highlight and audio times for the first few triggers
	var hl []time.Time
	var au []time.Time
	g.highlightHook = func(row, idx int) {
		if row == 0 {
			hl = append(hl, time.Now())
		}
	}
	g.SetPlayFunc(func(id string, vol float64, when ...float64) { au = append(au, time.Now()) })

	bpm := 120
	g.drum.SetBPM(bpm)
	waitApply(g, 150*time.Millisecond)

	g.drum.playPressed = true
	waitApply(g, 2500*time.Millisecond)

	if len(hl) < 2 || len(au) < 2 {
		t.Fatalf("insufficient events: hl=%d au=%d", len(hl), len(au))
	}
	// Compare the first aligned pair after start-up variability
	h := hl[1]
	aT := au[1]
	dt := aT.Sub(h).Seconds()
	// Expected: one subdivision at 120 BPM: (60/120)/32 = 1/64 s ≈ 15.625ms
	expected := (60.0 / float64(bpm)) / float64(32)
	diff := math.Abs(dt - expected)
	tol := 0.010 // 10ms tolerance for headless scheduling jitter
	if diff > tol {
		t.Fatalf("delay not within tolerance: got %.4fs want %.4fs (|diff|=%.4fs)", dt, expected, diff)
	}
}

// Rush cannot schedule earlier than 'now' due to audio backend delay clamping.
// Verify it never schedules later than the grid boundary and remains within 1 subdivision window.
func TestGrooveRushClampedWithinSubdivision(t *testing.T) {
	if os.Getenv("BPM_TIMING_TEST") != "1" {
		t.Skip("set BPM_TIMING_TEST=1")
	}
	logger := game_log.New(os.Stdout, game_log.LevelError)
	g := New(logger)
	g.Layout(800, 600)
	restore := SetInputForTest(func() (int, int) { return 0, 0 }, func(ebiten.MouseButton) bool { return false }, func(ebiten.Key) bool { return false }, func() []rune { return nil }, func() (float64, float64) { return 0, 0 }, func() (int, int) { return 800, 600 })
	defer restore()

	a, _ := buildEdge(g)
	if n, ok := g.graph.GetNodeByID(a.ID); ok {
		p := n.Params
		p.GrooveKind = "rush"
		p.GroovePct = 1.0
		g.graph.SetNodeParams(a.ID, p)
	}
	var hl []time.Time
	var au []time.Time
	g.highlightHook = func(row, idx int) {
		if row == 0 {
			hl = append(hl, time.Now())
		}
	}
	g.SetPlayFunc(func(id string, vol float64, when ...float64) { au = append(au, time.Now()) })

	bpm := 120
	g.drum.SetBPM(bpm)
	waitApply(g, 150*time.Millisecond)
	g.drum.playPressed = true
	waitApply(g, 1500*time.Millisecond)
	if len(hl) < 2 || len(au) < 2 {
		t.Fatalf("insufficient events: hl=%d au=%d", len(hl), len(au))
	}
	h := hl[1]
	aT := au[1]
	dt := aT.Sub(h).Seconds()
	// Rush should never push audio later than grid (no positive delay)
	if dt > 0.005 { // allow 5ms tolerance
		t.Fatalf("rush scheduled later than grid boundary: dt=%.4fs", dt)
	}
}
