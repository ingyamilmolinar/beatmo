package eventlogger

import (
	"sync"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// coalesceKinds enumerates the high-rate event kinds whose narrative is more
// useful as a "settled" line than as a per-tick stream. A 50ms BPM drag
// publishes ~50 EventBPMChange events; the user only wants to read one.
var coalesceKinds = map[hooks.Kind]struct{}{
	hooks.EventBPMChange:          {},
	hooks.EventMasterVolumeChange: {},
	hooks.EventEQBandChange:       {},
	hooks.EventNodeMoved:          {},
	hooks.EventInsertEffectParam:  {},
	hooks.EventRecordDropped:      {}, // drops in a burst → one line
	hooks.EventRowColorChanged:    {}, // color-wheel drag emits per-frame; coalesce
	// Verbose kinds are coalesced when delivered.
	hooks.EventCameraPan:    {},
	hooks.EventCameraZoom:   {},
	hooks.EventDragProgress: {},
}

// coalescer debounce-trails high-rate events: each new event for a coalesced
// Kind replaces any pending entry and rearms a per-Kind timer; the trailing
// entry is emitted when the window elapses with no further events. Other
// kinds bypass the coalescer entirely (see Logger.dispatch).
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
		// Emit outside the lock so a re-entrant Submit (rare, but possible
		// if emit triggers another publish) doesn't deadlock.
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
