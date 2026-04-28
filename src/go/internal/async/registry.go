package async

import (
	"context"
	"errors"
	"runtime"
	"sync"
)

// Registry holds named async pools that share a single concurrency
// budget. Use Get to obtain (or lazily create) a named pool — the
// registry rejects new pools whose worker count would push the total
// over Budget.
//
// One process-wide registry (DefaultRegistry) owns the budget so audio,
// hooks, recording, and observability all draw from the same pot. This
// is the only place that knows the system's total background-worker
// budget; individual subsystems just request a slice.
type Registry struct {
	mu     sync.Mutex
	pools  map[string]*Pool
	budget int
	used   int
	parent context.Context
}

// ErrBudgetExhausted is returned by Get when adding the requested pool
// would exceed the registry's total worker budget.
var ErrBudgetExhausted = errors.New("async: registry budget exhausted")

// NewRegistry creates a registry with an explicit budget. parent is the
// root context inherited by every pool created here.
func NewRegistry(parent context.Context, budget int) *Registry {
	if parent == nil {
		parent = context.Background()
	}
	if budget <= 0 {
		// Default leaves headroom for Ebitengine's 3 locked OS threads
		// (main/GLFW, render, game-thread) and oto's audio thread, but
		// also enforces a floor of 6 so the standard subsystem footprint
		// (recording.lifecycle, eventstream.persist, hooks.fanout=2,
		// audio.timers, ui.dialog) fits on small/test machines without
		// every caller having to fall back to private pools.
		budget = max(6, runtime.NumCPU()-4)
	}
	return &Registry{
		pools:  make(map[string]*Pool),
		budget: budget,
		parent: parent,
	}
}

// Get returns the pool registered under name, creating it on first call.
// If creating the pool would exceed Budget, returns ErrBudgetExhausted
// and the pool is not created. Subsequent Get calls with the same name
// return the existing pool regardless of opts (opts apply only on
// creation).
func (r *Registry) Get(name string, opts Options) (*Pool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.pools[name]; ok {
		return p, nil
	}
	want := opts.MaxConcurrent
	if want <= 0 {
		want = 1
	}
	if r.used+want > r.budget {
		return nil, ErrBudgetExhausted
	}
	if opts.Name == "" {
		opts.Name = name
	}
	p := NewPool(r.parent, opts)
	r.pools[name] = p
	r.used += want
	return p, nil
}

// MustGet panics if Get fails. Use only at process startup where a
// failure is unrecoverable and a panic at boot is acceptable.
func (r *Registry) MustGet(name string, opts Options) *Pool {
	p, err := r.Get(name, opts)
	if err != nil {
		panic("async.Registry.MustGet(" + name + "): " + err.Error())
	}
	return p
}

// Stats returns a snapshot of all named pools' stats keyed by pool name.
func (r *Registry) Stats() map[string]Stats {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]Stats, len(r.pools))
	for name, p := range r.pools {
		out[name] = p.Stats()
	}
	return out
}

// BudgetStats reports how the budget is allocated.
type BudgetStats struct {
	Budget int
	Used   int
	Pools  int
}

// BudgetStats returns budget/used/pool counters.
func (r *Registry) BudgetStats() BudgetStats {
	r.mu.Lock()
	defer r.mu.Unlock()
	return BudgetStats{Budget: r.budget, Used: r.used, Pools: len(r.pools)}
}

// Release closes the pool registered under name and frees its worker
// slots back to the budget. No-op if name is not registered. Use for
// transient pools (e.g., ad-hoc work in tests) so the registry's budget
// doesn't leak. Long-lived subsystem pools should generally NOT be
// released — Shutdown handles them at process exit.
func (r *Registry) Release(name string) error {
	r.mu.Lock()
	p, ok := r.pools[name]
	if !ok {
		r.mu.Unlock()
		return nil
	}
	delete(r.pools, name)
	// Pool.workers reflects the configured count, which is what we
	// charged against the budget at creation. Refund the same amount.
	r.used -= p.workers
	if r.used < 0 {
		r.used = 0
	}
	r.mu.Unlock()
	return p.Close()
}

// Shutdown drains every pool. Returns the join of any per-pool Close
// errors.
func (r *Registry) Shutdown() error {
	r.mu.Lock()
	pools := make([]*Pool, 0, len(r.pools))
	for _, p := range r.pools {
		pools = append(pools, p)
	}
	r.mu.Unlock()
	var errs []error
	for _, p := range pools {
		if err := p.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// defaultRegistry is the lazily-initialized process-wide registry.
var (
	defaultRegistryOnce sync.Once
	defaultRegistry     *Registry
)

// DefaultRegistry returns the singleton process-wide registry.
func DefaultRegistry() *Registry {
	defaultRegistryOnce.Do(func() {
		defaultRegistry = NewRegistry(context.Background(), 0)
	})
	return defaultRegistry
}
