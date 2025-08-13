package engine

import (
	"context"
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
	ctx    context.Context
	cancel context.CancelFunc
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

	sched.OnTick = func(step int) {
		select {
		case e.Events <- Event{Step: step}:
		default:
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
