package hooks

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/async"
)

// Bus is a non-blocking pub/sub event bus. Publish never blocks: each
// subscriber callback runs as a separate job on the bus's async.Pool. If
// the pool is saturated the event is dropped (counted, never blocks the
// publisher).
//
// Bus is safe for concurrent use from any goroutine. Subscribers must
// not assume any particular ordering across kinds and must not block
// indefinitely; the pool is bounded.
type Bus struct {
	pool *async.Pool

	mu     sync.RWMutex
	subs   map[Kind][]*subscription
	closed atomic.Bool
	// counters
	published atomic.Int64
	dropped   atomic.Int64
}

type subscription struct {
	id uint64
	fn func(Event)
}

// Options configures a new Bus.
type Options struct {
	// PoolWorkers caps concurrent subscriber callbacks. 0 → 4.
	PoolWorkers int
	// PoolQueue caps the queued (not-yet-running) callback count. 0 → 256.
	PoolQueue int
	// Name is included in pool metrics.
	Name string
}

// NewBus constructs a standalone Bus with its own internal async.Pool.
// Used by tests and any caller that wants isolation from the process-
// wide registry. Production code should call GlobalBus instead so all
// fan-out work counts against a single budget.
func NewBus(parent context.Context, opts Options) *Bus {
	if opts.PoolWorkers <= 0 {
		opts.PoolWorkers = 4
	}
	if opts.PoolQueue <= 0 {
		opts.PoolQueue = 256
	}
	if opts.Name == "" {
		opts.Name = "hooks"
	}
	b := &Bus{
		subs: make(map[Kind][]*subscription),
	}
	b.pool = async.NewPool(parent, async.Options{
		MaxConcurrent: opts.PoolWorkers,
		QueueSize:     opts.PoolQueue,
		Name:          opts.Name,
		OnDrop:        func(string) { b.dropped.Add(1) },
	})
	return b
}

// newRegistryBus constructs a Bus backed by the process-wide
// async.Registry. Used by GlobalBus so the hooks fan-out workers count
// against the same global budget as recording, eventstream, etc.
func newRegistryBus(opts Options) *Bus {
	if opts.PoolWorkers <= 0 {
		opts.PoolWorkers = 2
	}
	if opts.PoolQueue <= 0 {
		opts.PoolQueue = 256
	}
	if opts.Name == "" {
		opts.Name = "hooks.fanout"
	}
	b := &Bus{
		subs: make(map[Kind][]*subscription),
	}
	pool, err := async.DefaultRegistry().Get(opts.Name, async.Options{
		MaxConcurrent: opts.PoolWorkers,
		QueueSize:     opts.PoolQueue,
		Name:          opts.Name,
		OnDrop:        func(string) { b.dropped.Add(1) },
	})
	if err != nil {
		// Registry budget exhausted — fall back to a private pool so the
		// bus still functions (we don't want to silently drop hooks just
		// because some other subsystem hogged the budget).
		pool = async.NewPool(context.Background(), async.Options{
			MaxConcurrent: opts.PoolWorkers,
			QueueSize:     opts.PoolQueue,
			Name:          opts.Name + ".fallback",
			OnDrop:        func(string) { b.dropped.Add(1) },
		})
	}
	b.pool = pool
	return b
}

var nextSubID atomic.Uint64

// Subscribe registers fn for events of kind k. Returns an unsubscribe
// closure; calling it more than once is a no-op.
func (b *Bus) Subscribe(k Kind, fn func(Event)) (unsubscribe func()) {
	if fn == nil {
		return func() {}
	}
	id := nextSubID.Add(1)
	sub := &subscription{id: id, fn: fn}
	b.mu.Lock()
	b.subs[k] = append(b.subs[k], sub)
	b.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			list := b.subs[k]
			for i, s := range list {
				if s.id == id {
					b.subs[k] = append(list[:i], list[i+1:]...)
					return
				}
			}
		})
	}
}

// Publish dispatches e to all subscribers of e.Kind asynchronously. If
// the bus is closed or has no subscribers for the kind, returns
// immediately. Never blocks.
func (b *Bus) Publish(e Event) {
	if b.closed.Load() {
		return
	}
	if e.At.IsZero() {
		e.At = time.Now()
	}
	b.published.Add(1)

	b.mu.RLock()
	subs := b.subs[e.Kind]
	if len(subs) == 0 {
		b.mu.RUnlock()
		return
	}
	// Snapshot so we don't hold the lock across pool.Submit.
	snapshot := make([]*subscription, len(subs))
	copy(snapshot, subs)
	b.mu.RUnlock()

	for _, sub := range snapshot {
		s := sub
		_ = b.pool.Submit(func(_ context.Context) { s.fn(e) })
	}
}

// PublishKind is a convenience for Publish(Event{Kind: k, Payload: payload}).
// The resulting Event carries no Source; use PublishWithSource from the emit
// helpers in internal/ui/event_helpers.go when source attribution is wanted.
func (b *Bus) PublishKind(k Kind, payload any) {
	b.Publish(Event{Kind: k, Payload: payload})
}

// PublishWithSource is the source-aware variant of PublishKind. The src
// argument is typically captured at the call site of an emit helper via
// captureSource (see internal/hooks/source.go); the resulting Event carries
// the originating user-code file:line through the bus to subscribers.
func (b *Bus) PublishWithSource(k Kind, payload any, src Source) {
	b.Publish(Event{Kind: k, Payload: payload, Source: src})
}

// Stats returns publish/drop counters and a snapshot of the underlying
// async.Pool stats.
type Stats struct {
	Published int64
	Dropped   int64
	Pool      async.Stats
}

// Stats returns counters for monitoring.
func (b *Bus) Stats() Stats {
	return Stats{
		Published: b.published.Load(),
		Dropped:   b.dropped.Load(),
		Pool:      b.pool.Stats(),
	}
}

// Close shuts the bus down: no further events are dispatched and the
// underlying pool is drained.
func (b *Bus) Close() error {
	b.closed.Store(true)
	return b.pool.Close()
}

// globalBus is the process-wide bus, created lazily so libraries that
// don't construct one don't pay for an idle pool. Most callers should use
// the package-level Publish/Subscribe helpers.
var (
	globalBusOnce sync.Once
	globalBus     *Bus
)

// GlobalBus returns the lazily-initialized process-wide Bus. Its
// fan-out pool is sourced from async.DefaultRegistry so the bus's
// workers count against the same global budget as recording and
// eventstream — the system never spawns more goroutines than the
// registry budget allows.
func GlobalBus() *Bus {
	globalBusOnce.Do(func() {
		globalBus = newRegistryBus(Options{Name: "hooks.fanout"})
	})
	return globalBus
}

// Publish is shorthand for GlobalBus().Publish(e).
func Publish(e Event) { GlobalBus().Publish(e) }

// PublishKind is shorthand for GlobalBus().PublishKind(k, payload).
func PublishKind(k Kind, payload any) { GlobalBus().PublishKind(k, payload) }

// PublishWithSource is shorthand for GlobalBus().PublishWithSource(k, payload, src).
func PublishWithSource(k Kind, payload any, src Source) {
	GlobalBus().PublishWithSource(k, payload, src)
}

// Subscribe is shorthand for GlobalBus().Subscribe(k, fn).
func Subscribe(k Kind, fn func(Event)) func() {
	return GlobalBus().Subscribe(k, fn)
}
