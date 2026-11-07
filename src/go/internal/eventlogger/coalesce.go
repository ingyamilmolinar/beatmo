package eventlogger

import (
	"sync"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// High-rate kinds whose narrative reads better as one settled line: a BPM drag publishes dozens of events.
var coalesceKinds = map[hooks.Kind]struct{}{
	hooks.EventBPMChange:          {},
	hooks.EventMasterVolumeChange: {},
	hooks.EventEQBandChange:       {},
	hooks.EventNodeMoved:          {},
	hooks.EventInsertEffectParam:  {},
	hooks.EventRecordDropped:      {}, // drops in a burst → one line
	hooks.EventRowColorChanged:    {}, // color-wheel drag emits per-frame; coalesce
	hooks.EventScroll:             {}, // flick + momentum → one trailing line
	hooks.EventSearchChanged:      {}, // per-keystroke typing → one trailing settled query
	// Verbose kinds are coalesced when delivered.
	hooks.EventCameraPan:    {},
	hooks.EventCameraZoom:   {},
	hooks.EventDragProgress: {},
}

// Debounce-trailing: each event replaces the pending one for its Kind and rearms that Kind's timer; the last is emitted
// when the window passes quietly. Other kinds bypass it.
type coalescer struct {
	window time.Duration
	emit   func(hooks.Event)

	mu      sync.Mutex
	pending map[hooks.Kind]*pendingEntry
}

type pendingEntry struct {
	ev    hooks.Event
	timer *time.Timer
}

func newCoalescer(window time.Duration, emit func(hooks.Event)) *coalescer {
	if window <= 0 {
		window = 250 * time.Millisecond
	}
	return &coalescer{
		window:  window,
		emit:    emit,
		pending: make(map[hooks.Kind]*pendingEntry),
	}
}

// IsCoalesced reports whether k is debounce-trailed by this coalescer.
func (c *coalescer) IsCoalesced(k hooks.Kind) bool {
	if c == nil {
		return false
	}
	_, ok := coalesceKinds[k]
	return ok
}

// Submit replaces any pending entry for e.Kind and rearms the per-Kind
// timer. If no entry exists, schedules one. Safe for concurrent callers.
func (c *coalescer) Submit(e hooks.Event) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	kind := e.Kind
	if entry, ok := c.pending[kind]; ok {
		entry.ev = e
		entry.timer.Reset(c.window)
		return
	}
	entry := &pendingEntry{ev: e}
	entry.timer = time.AfterFunc(c.window, func() {
		c.mu.Lock()
		latest, ok := c.pending[kind]
		if !ok {
			c.mu.Unlock()
			return
		}
		ev := latest.ev
		delete(c.pending, kind)
		c.mu.Unlock()
		// Outside the lock, so a re-entrant Submit (emit triggering a publish) cannot deadlock.
		c.emit(ev)
	})
	c.pending[kind] = entry
}

// FlushAll synchronously emits every pending coalesced entry. Used at
// shutdown and by tests that want deterministic output without waiting on
// real timers.
func (c *coalescer) FlushAll() {
	if c == nil {
		return
	}
	c.mu.Lock()
	type pair struct {
		kind hooks.Kind
		ev   hooks.Event
	}
	var emitNow []pair
	for k, entry := range c.pending {
		entry.timer.Stop()
		emitNow = append(emitNow, pair{k, entry.ev})
	}
	c.pending = make(map[hooks.Kind]*pendingEntry)
	c.mu.Unlock()
	// Emit outside the lock.
	for _, p := range emitNow {
		c.emit(p.ev)
	}
}
