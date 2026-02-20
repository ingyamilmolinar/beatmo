package engine

import (
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

func testLogger() *game_log.Logger {
	return game_log.New(io.Discard, game_log.LevelError)
}

func TestEngineStartStop(t *testing.T) {
	e := New(testLogger())
	defer e.Close()

	// Before Start, scheduler is not running; Progress should be 0.
	if p := e.Progress(); p != 0 {
		t.Fatalf("expected progress 0 before start, got %f", p)
	}

	e.Start()

	// Let the engine tick a few times so the scheduler fires.
	time.Sleep(40 * time.Millisecond)

	// After Start + ticks, Progress should be non-zero (scheduler is running and
	// last is set after the first Tick fires).
	if p := e.Progress(); p == 0 {
		// Progress can legitimately be 0 right at the tick boundary, so we
		// give it one more chance after a short sleep.
		time.Sleep(20 * time.Millisecond)
		if p2 := e.Progress(); p2 == 0 {
			t.Fatalf("expected non-zero progress after start, got 0 twice")
		}
	}

	e.Stop()

	// After Stop, Progress should return 0 because last is reset to zero time.
	if p := e.Progress(); p != 0 {
		t.Fatalf("expected progress 0 after stop, got %f", p)
	}
}

func TestEngineSetBPM(t *testing.T) {
	e := New(testLogger())
	defer e.Close()

	// Default BPM is 120.
	if bpm := e.BPM(); bpm != 120 {
		t.Fatalf("expected default BPM 120, got %d", bpm)
	}

	e.SetBPM(90)
	if bpm := e.BPM(); bpm != 90 {
		t.Fatalf("expected BPM 90 after SetBPM, got %d", bpm)
	}

	e.SetBPM(180)
	if bpm := e.BPM(); bpm != 180 {
		t.Fatalf("expected BPM 180 after SetBPM, got %d", bpm)
	}
}

func TestEngineBeatLength(t *testing.T) {
	e := New(testLogger())
	defer e.Close()

	if bl := e.BeatLength(); bl != 16 {
		t.Fatalf("expected default beat length 16, got %d", bl)
	}
}

func TestEngineProgress(t *testing.T) {
	e := New(testLogger())
	defer e.Close()

	// Progress before start should be 0.
	if p := e.Progress(); p != 0 {
		t.Fatalf("expected progress 0 before start, got %f", p)
	}

	e.Start()
	// Allow scheduler to tick at least once.
	time.Sleep(40 * time.Millisecond)

	p := e.Progress()
	// Progress should be a non-negative value. After the first tick it will be
	// somewhere in [0, 1). We just verify the delegation doesn't panic and
	// returns something reasonable.
	if p < 0 || p > 1.0 {
		t.Fatalf("expected progress in [0, 1), got %f", p)
	}

	e.Stop()
}

func TestEngineSubscribe(t *testing.T) {
	e := New(testLogger())
	defer e.Close()

	sub := e.Subscribe()

	e.Start()
	defer e.Stop()

	// Wait for at least one event on the subscription channel. The engine's
	// run loop ticks every 16ms and at 120 BPM a beat fires every 500ms, so
	// the first tick-on-start fires immediately.
	select {
	case evt := <-sub:
		// Step should be a non-negative value.
		if evt.Step < 0 {
			t.Fatalf("unexpected negative step: %d", evt.Step)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for subscribe event")
	}
}

func TestEngineSubscribeMultiple(t *testing.T) {
	e := New(testLogger())
	defer e.Close()

	sub1 := e.Subscribe()
	sub2 := e.Subscribe()

	e.Start()
	defer e.Stop()

	// Both subscribers should receive events.
	for i, ch := range []<-chan Event{sub1, sub2} {
		select {
		case <-ch:
			// ok
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("subscriber %d timed out waiting for event", i)
		}
	}
}

func TestEngineEventsChannel(t *testing.T) {
	e := New(testLogger())
	defer e.Close()

	e.Start()
	defer e.Stop()

	// The built-in Events channel should also receive tick events.
	select {
	case evt := <-e.Events:
		if evt.Step < 0 {
			t.Fatalf("unexpected negative step: %d", evt.Step)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for event on Events channel")
	}
}

func TestEngineNodeDeletionHook(t *testing.T) {
	e := New(testLogger())
	defer e.Close()

	// Add a regular node to the engine's graph.
	n0 := e.Graph.AddNode(0, 0, model.NodeTypeRegular)
	e.Graph.StartNodeID = n0

	// Set up predictor with a single-node looping path.
	path := []model.BeatInfo{{NodeID: n0, NodeType: model.NodeTypeRegular}}
	nodes := map[model.NodeID]model.Node{n0: e.Graph.Nodes[n0]}

	e.Predictor.SetPaths([][]model.BeatInfo{path}, []bool{true}, []int{0}, nodes)
	e.Predictor.Ensure(4)

	// The node should be audible at index 0.
	if !e.Predictor.AudibleAt(0, 0) {
		t.Fatal("expected node audible at (0, 0) before deletion")
	}

	// Remove the node via graph (which triggers the node-changed hook,
	// calling Predictor.DeleteNode).
	e.Graph.RemoveNode(n0)

	// Re-ensure after deletion; the node should no longer be audible.
	e.Predictor.Ensure(4)
	if e.Predictor.AudibleAt(0, 0) {
		t.Fatal("expected node not audible at (0, 0) after deletion via hook")
	}
}

func TestEngineNodeUpdateHook(t *testing.T) {
	e := New(testLogger())
	defer e.Close()

	// Add a regular node.
	_ = e.Graph.AddNode(0, 0, model.NodeTypeRegular)

	// The hook should have been called by AddNode, updating the predictor's
	// node snapshot. Verify the predictor marked itself dirty.
	if !e.Predictor.PredDirtyForTest() {
		t.Fatal("expected predictor dirty after AddNode hook")
	}
}

func TestEngineStartStopIdempotent(t *testing.T) {
	e := New(testLogger())
	defer e.Close()

	// Multiple starts should not panic.
	e.Start()
	e.Start()
	time.Sleep(20 * time.Millisecond)

	// Multiple stops should not panic.
	e.Stop()
	e.Stop()

	if p := e.Progress(); p != 0 {
		t.Fatalf("expected progress 0 after double stop, got %f", p)
	}
}

func TestEngineSetBPMWhileRunning(t *testing.T) {
	e := New(testLogger())
	defer e.Close()

	e.Start()
	defer e.Stop()

	// Let the scheduler run a bit.
	time.Sleep(30 * time.Millisecond)

	// Changing BPM mid-playback should not panic.
	e.SetBPM(60)
	if bpm := e.BPM(); bpm != 60 {
		t.Fatalf("expected BPM 60 while running, got %d", bpm)
	}

	e.SetBPM(200)
	if bpm := e.BPM(); bpm != 200 {
		t.Fatalf("expected BPM 200 while running, got %d", bpm)
	}
}

func TestEngineCloseStopsRunLoop(t *testing.T) {
	e := New(testLogger())

	sub := e.Subscribe()
	e.Start()

	// Drain at least one event to confirm the run loop is active.
	select {
	case <-sub:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("no event before close")
	}

	e.Close()

	// After Close, the run loop goroutine should exit. No further events
	// should arrive. We use a short window to verify.
	var count int64
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case _, ok := <-sub:
				if !ok {
					return
				}
				atomic.AddInt64(&count, 1)
			case <-time.After(50 * time.Millisecond):
				return
			}
		}
	}()
	<-done

	// A few events may have been in-flight before the goroutine noticed the
	// context cancellation, but we should not see a sustained stream.
	if c := atomic.LoadInt64(&count); c > 3 {
		t.Fatalf("expected very few events after Close, got %d", c)
	}
}

func TestEngineGraphAndPredictorNotNil(t *testing.T) {
	e := New(testLogger())
	defer e.Close()

	if e.Graph == nil {
		t.Fatal("expected Graph to be non-nil")
	}
	if e.Predictor == nil {
		t.Fatal("expected Predictor to be non-nil")
	}
}
