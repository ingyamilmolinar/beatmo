package hooks

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/async"
)

// Publish never blocks: each subscriber runs as a job on the bus's pool, and a saturated pool drops (counted).
// Subscribers must not assume ordering across kinds or block indefinitely.
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

// A standalone pool, for isolation; production uses GlobalBus, so fan-out counts against the one budget.
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
		// Budget exhausted: a private pool, so hooks are never silently dropped.
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

// No Source: the emit helpers use PublishWithSource when attribution is wanted.
func (b *Bus) PublishKind(k Kind, payload any) {
	b.Publish(Event{Kind: k, Payload: payload})
}

// src is captured at the emit helper's call site, so the originating file:line reaches subscribers.
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

// Lazy, so a library that never publishes pays for no idle pool.
var (
	globalBusOnce sync.Once
	globalBus     *Bus
)

// Its fan-out pool comes from async.DefaultRegistry, the same budget as everything else.
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
