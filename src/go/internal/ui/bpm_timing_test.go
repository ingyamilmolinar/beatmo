package ui

import (
	"os"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

// TestBPMFunctional60 verifies that with BPM=60 a simple 1-beat-per-side
// square triggers audible beats at approximately 1 Hz. The grid has 32
// subdivisions per beat, so one beat equals 32 grid units. We construct a
// 1x1-beat rectangle and measure the time between successive row-0 note
// triggers. The tolerance is tight enough to catch drift and lag.
func TestBPMFunctional60(t *testing.T) {
	if os.Getenv("BPM_TIMING_TEST") != "1" {
		t.Skip("Set BPM_TIMING_TEST=1 to enable this functional timing test.")
	}
	logger := game_log.New(os.Stdout, game_log.LevelError)
	g := New(logger)
	g.Layout(800, 600)

	// Freeze input to avoid incidental wheel/drag events in real-Ebiten mode.
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	SetDefaultStartForTest(false)

	// Build a 1x1-beat rectangle; corners separated by 1 beat at 60 BPM.
	beat := 32
	g.pendingStartRow = 0
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n1 := g.tryAddNode(beat, 0, model.NodeTypeRegular)
	n2 := g.tryAddNode(beat, beat, model.NodeTypeRegular)
	n3 := g.tryAddNode(0, beat, model.NodeTypeRegular)
	g.addEdge(n0, n1)
	g.addEdge(n1, n2)
	g.addEdge(n2, n3)
	g.addEdge(n3, n0)
	g.pendingStartRow = -1
	g.updateBeatInfos()

	// Capture highlight events (regular nodes only) with wall-clock timestamps.
	var times []time.Time
	g.highlightHook = func(row, idx int) {
		if row == 0 {
			times = append(times, time.Now())
		}
	}

	// Set BPM=60 (1 beat/sec), apply it, then start playback.
	g.drum.SetBPM(60)
	applyUntil := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(applyUntil) {
		_ = g.Update()
		time.Sleep(17 * time.Millisecond)
	}

	g.drum.playPressed = true
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		_ = g.Update()
		time.Sleep(17 * time.Millisecond)
	}

	if len(times) < 3 {
		t.Fatalf("insufficient events captured: %d", len(times))
	}
	var intervals []time.Duration
	for i := 2; i < len(times); i++ {
		intervals = append(intervals, times[i].Sub(times[i-1]))
	}
	if len(intervals) == 0 {
		t.Fatalf("no intervals computed; times=%v", times)
	}
	for i, d := range intervals {
		low := 998*time.Millisecond + 500*time.Microsecond
		high := 1001*time.Millisecond + 500*time.Microsecond
		if d < low || d > high { // ±1.5ms
			t.Fatalf("interval %d not ~1s (±1.5ms): got %v (events=%d)", i, d, len(times))
		}
	}
}

// TestBPMStressShortSegmentsHighBPM stress tests timing with very short
// segments (1/32 beat) at high BPM and multiple concurrent rows. It verifies
// that audio callbacks occur within 25ms of the expected schedule. This test
// is opt-in to keep default runs fast and portable; enable with
// BPM_TIMING_TEST=1.
func TestBPMStressShortSegmentsHighBPM(t *testing.T) {
	if os.Getenv("BPM_TIMING_TEST") != "1" {
		t.Skip("Set BPM_TIMING_TEST=1 to enable this stress timing test.")
	}
	logger := game_log.New(os.Stdout, game_log.LevelError)
	g := New(logger)
	g.Layout(800, 600)

	// Build 3 rows, each a tight 1x1 rectangle in smallest subdivisions.
	// Distance between consecutive regular corners = 1/32 beat.
	beatUnit := 1 // smallest subdivision
	buildRect := func(row, x, y int) {
		g.pendingStartRow = row
		n0 := g.tryAddNode(x, y, model.NodeTypeRegular)
		n1 := g.tryAddNode(x+beatUnit, y, model.NodeTypeRegular)
		n2 := g.tryAddNode(x+beatUnit, y+beatUnit, model.NodeTypeRegular)
		n3 := g.tryAddNode(x, y+beatUnit, model.NodeTypeRegular)
		g.addEdge(n0, n1)
		g.addEdge(n1, n2)
		g.addEdge(n2, n3)
		g.addEdge(n3, n0)
		g.pendingStartRow = -1
	}

	// Ensure at least 3 rows exist.
	for len(g.drum.Rows) < 3 {
		g.drum.AddRow()
	}
	buildRect(0, 0, 0)
	buildRect(1, 5, 0)
	buildRect(2, 10, 0)
	g.updateBeatInfos()

	// Assign per-row instrument IDs so we can attribute callbacks.
	g.drum.Rows[0].Instrument = "row0"
	g.drum.Rows[1].Instrument = "row1"
	g.drum.Rows[2].Instrument = "row2"

	// High BPM to stress scheduling.
	bpm := 240 // 1 beat = 250ms, 1/32 beat ≈ 7.8125ms
	g.drum.SetBPM(bpm)
	// Let BPM apply.
	applyUntil := time.Now().Add(100 * time.Millisecond)
	for time.Now().Before(applyUntil) {
		_ = g.Update()
		time.Sleep(5 * time.Millisecond)
	}

	// Capture per-row audio callback times.
	type rec struct {
		id string
		t  time.Time
	}
	var times []rec
	g.SetPlayFunc(func(id string, vol float64, when ...float64) {
		times = append(times, rec{id: id, t: time.Now()})
	})

	// Start pulses for each row and run for a short window.
	g.playing = true
	g.spawnPulseFromRow(0, 0)
	g.spawnPulseFromRow(1, 0)
	g.spawnPulseFromRow(2, 0)

	deadline := time.Now().Add(600 * time.Millisecond)
	for time.Now().Before(deadline) {
		_ = g.Update()
		time.Sleep(2 * time.Millisecond)
	}

	// Group by instrument and verify interval <= expected + 25ms tolerance.
	got := map[string][]time.Time{}
	for _, r := range times {
		got[r.id] = append(got[r.id], r.t)
	}
	if len(got) == 0 {
		t.Fatalf("no audio callbacks captured")
	}
	expected := (60_000.0 / float64(bpm)) / 32.0 // ms per corner at 1/32 beat
	tol := 1.8                                   // ms (tight but robust)
	for id, arr := range got {
		if len(arr) < 3 {
			t.Fatalf("insufficient events for %s: %d", id, len(arr))
		}
		for i := 1; i < len(arr); i++ {
			dt := arr[i].Sub(arr[i-1]).Seconds() * 1000
			diff := dt - expected
			if diff < 0 {
				diff = -diff
			}
			if diff > tol {
				t.Fatalf("row %s interval drift: got=%.2fms expected=%.2fms tol=%.2fms", id, dt, expected, tol)
			}
		}
	}
}

// TestHighlightAudioSync25ms compares per-node highlight timestamps against
// audio callback timestamps for a simple 1-beat-per-edge loop and asserts the
// difference stays within 25ms. Validates UI → audio queuing path.
func TestHighlightAudioSync25ms(t *testing.T) {
	if os.Getenv("BPM_TIMING_TEST") != "1" {
		t.Skip("Set BPM_TIMING_TEST=1 to enable this timing test.")
	}
	logger := game_log.New(os.Stdout, game_log.LevelError)
	g := New(logger)
	g.Layout(800, 600)

	// Freeze input for stability in headless.
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	SetDefaultStartForTest(false)

	// Rectangle with 1-beat edges.
	beat := 32
	g.pendingStartRow = 0
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n1 := g.tryAddNode(beat, 0, model.NodeTypeRegular)
	n2 := g.tryAddNode(beat, beat, model.NodeTypeRegular)
	n3 := g.tryAddNode(0, beat, model.NodeTypeRegular)
	g.addEdge(n0, n1)
	g.addEdge(n1, n2)
	g.addEdge(n2, n3)
	g.addEdge(n3, n0)
	g.pendingStartRow = -1
	g.updateBeatInfos()

	// Capture highlight and audio callbacks with wall-clock timestamps.
	var hlTimes []time.Time
	var auTimes []time.Time
	g.highlightHook = func(row, idx int) {
		if row == 0 {
			hlTimes = append(hlTimes, time.Now())
		}
	}
	g.SetPlayFunc(func(id string, vol float64, when ...float64) { auTimes = append(auTimes, time.Now()) })

	// BPM 60, let it apply.
	g.drum.SetBPM(60)
	applyUntil := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(applyUntil) {
		_ = g.Update()
		time.Sleep(17 * time.Millisecond)
	}

	// Start and run for ~6 seconds to gather several events.
	g.drum.playPressed = true
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		_ = g.Update()
		time.Sleep(17 * time.Millisecond)
	}

	// Align by trimming to the shortest length and skipping the first event to
	// avoid startup variability.
	if len(hlTimes) < 3 || len(auTimes) < 3 {
		t.Fatalf("insufficient events: highlights=%d audio=%d", len(hlTimes), len(auTimes))
	}
	n := len(hlTimes)
	if len(auTimes) < n {
		n = len(auTimes)
	}
	hl := hlTimes[1:n]
	au := auTimes[1:n]

	tol := 0.05 // ms (50µs)
	for i := 0; i < len(hl) && i < len(au); i++ {
		diff := au[i].Sub(hl[i]).Seconds() * 1000
		if diff < 0 {
			diff = -diff
		}
		if diff > tol {
			t.Fatalf("highlight-audio mismatch at %d: |%.2fms| > %.2fms (hl=%v au=%v)", i, diff, tol, hl[i], au[i])
		}
	}
}
