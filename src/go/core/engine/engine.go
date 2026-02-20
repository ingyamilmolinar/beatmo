package engine

import (
	"context"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/beat"
	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
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
	logger *game_log.Logger
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
	sched := beat.NewScheduler(logger)
	ctx, cancel := context.WithCancel(context.Background())

	e := &Engine{
		Graph:  graph,
		sched:  sched,
		Events: make(chan Event, 16),
		ctx:    ctx,
		cancel: cancel,
		logger: logger,
	}

	// Engine predictor is the single source of truth for prediction buffers and
	// contexts across desktop/WASM and tests.
	e.Predictor = NewPredictor(graph, logger)
	graph.SetNodeChangedHook(func(id model.NodeID) {
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
					// [PERF/ENGINE] avg=.. max=.. count=.. (debug-only)
					if e.logger != nil {
						e.logger.Debugf("[PERF/ENGINE] ticker avg=%s max=%s count=%d", avg.String(), e.tickMax.String(), e.tickCount)
					}
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
func (e *Engine) Close() {
	// Stop engine loop and background predictor precompute if active.
	e.cancel()
	if e.Predictor != nil {
		e.Predictor.StopBackground()
	}
}

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
