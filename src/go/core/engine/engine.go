package engine

import (
	"context"
	"os"
	"time"

	"github.com/ingyamilmolinar/tunkul/core/beat"
	"github.com/ingyamilmolinar/tunkul/core/model"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

// Event represents a tick from the game engine.
type Event struct {
	Step int
}

const tickInterval = 16 * time.Millisecond

// Engine encapsulates the core game logic and runs it on its own goroutine.
type Engine struct {
	Graph  *model.Graph
	sched  *beat.Scheduler
	Events chan Event
	subs   []chan Event
	ctx    context.Context
	cancel context.CancelFunc
	// Predictor holds prediction buffers/contexts outside the UI.
	Predictor *Predictor

	// perf (best-effort): engine ticker interval stats for diagnostics
	lastTick  time.Time
	tickCount int64
	tickSum   time.Duration
	tickMax   time.Duration
	tickLogAt time.Time
}

// New creates a new Engine instance and starts its run loop.
func New(logger *game_log.Logger) *Engine {
	graph := model.NewGraph(logger)
	sched := beat.NewScheduler()
	ctx, cancel := context.WithCancel(context.Background())

	e := &Engine{
		Graph:  graph,
		sched:  sched,
		Events: make(chan Event, 16),
		ctx:    ctx,
		cancel: cancel,
	}

	// Initialize predictor bound to this engine's graph unless explicitly
	// disabled via env. Default is enabled to make engine the single source
	// of truth for prediction buffers and contexts.
	if os.Getenv("USE_ENGINE_PREDICTOR") != "0" { // default ON
		e.Predictor = NewPredictor(graph)
	}
	graph.SetNodeChangedHook(func(id model.NodeID) {
		if e.Predictor == nil {
			return
		}
		if node, ok := graph.GetNodeByID(id); ok {
			e.Predictor.UpdateNode(id, node)
		} else {
			e.Predictor.DeleteNode(id)
		}
	})

	sched.OnTick = func(step int) {
		evt := Event{Step: step}
		select {
		case e.Events <- evt:
		default:
		}
		// broadcast to subscribers non-blockingly
		for _, ch := range e.subs {
			select {
			case ch <- evt:
			default:
			}
		}
	}

	go e.run()
	return e
}

func (e *Engine) run() {
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			// Perf: measure ticker jitter and log occasionally
			now := time.Now()
			if !e.lastTick.IsZero() {
				dt := now.Sub(e.lastTick)
				e.tickCount++
				e.tickSum += dt
				if dt > e.tickMax {
					e.tickMax = dt
				}
				if e.tickLogAt.IsZero() {
					e.tickLogAt = now.Add(2 * time.Second)
				}
				if now.After(e.tickLogAt) {
					avg := time.Duration(0)
					if e.tickCount > 0 {
						avg = time.Duration(int64(e.tickSum) / e.tickCount)
					}
					// Log to stdout; the app’s logger isn’t wired here.
					// This is for side-by-side desktop/wasm comparison.
					// Format is stable so tests can grep if desired.
					// PERF engine: avg=.. max=.. count=..
					_ = os.Stderr // avoid linter complaining in non-test builds
					// Use print to avoid import cycles with logger.
					println("PERF engine ticker:", "avg=", avg.String(), "max=", e.tickMax.String(), "count=", e.tickCount)
					e.tickCount, e.tickSum, e.tickMax = 0, 0, 0
					e.tickLogAt = now.Add(2 * time.Second)
				}
			}
			e.lastTick = now
			e.sched.Tick()
		case <-e.ctx.Done():
			return
		}
	}
}

// Start begins the scheduler.
func (e *Engine) Start() { e.sched.Start() }

// Stop stops the scheduler.
func (e *Engine) Stop() { e.sched.Stop() }

// SetBPM updates the scheduler BPM.
func (e *Engine) SetBPM(bpm int) { e.sched.SetBPM(bpm) }

// BPM returns the current scheduler BPM.
func (e *Engine) BPM() int { return e.sched.BPM }

// Close terminates the engine goroutine.
func (e *Engine) Close() { e.cancel() }

// BeatLength exposes the scheduler's beat length.
func (e *Engine) BeatLength() int { return e.sched.BeatLength }

// Progress exposes the scheduler's current beat progress.
func (e *Engine) Progress() float64 { return e.sched.Progress() }

// Subscribe returns a channel that receives tick events in parallel to Events.
// The returned channel is buffered; delivery is best-effort and may drop
// events if the receiver falls behind.
func (e *Engine) Subscribe() <-chan Event {
	ch := make(chan Event, 16)
	e.subs = append(e.subs, ch)
	return ch
}
