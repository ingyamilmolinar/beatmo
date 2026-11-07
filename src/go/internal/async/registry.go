package async

import (
	"context"
	"errors"
	"runtime"
	"sync"
)

// Named pools sharing ONE worker budget (DefaultRegistry is process-wide), so every subsystem draws from the same pot;
// a pool that would exceed Budget is refused.
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

// MUST cover the whole standing-pool footprint (CLAUDE.md's table, pinned by registry_footprint_test.go). A lower floor
// saves nothing (fallback callers allocate the same workers privately) and breaks the callers with no fallback:
// ui.dialog (WAV upload fails) and recording.lifecycle (MustGet panics).
const minDefaultBudget = 13

// NewRegistry creates a registry with an explicit budget. parent is the
// root context inherited by every pool created here.
func NewRegistry(parent context.Context, budget int) *Registry {
	if parent == nil {
		parent = context.Background()
	}
	if budget <= 0 {
		// Leaves Ebitengine's locked OS threads and oto's audio thread their cores, with a floor so the standard footprint fits
		// small machines without private-pool fallbacks.
		budget = max(minDefaultBudget, runtime.NumCPU()-4)
	}
	return &Registry{
		pools:  make(map[string]*Pool),
		budget: budget,
		parent: parent,
	}
}

// opts apply only on creation; a later Get returns the existing pool whatever they say.
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

// For transient pools (tests), so the budget never leaks; standing pools are left to Shutdown.
func (r *Registry) Release(name string) error {
	r.mu.Lock()
	p, ok := r.pools[name]
	if !ok {
		r.mu.Unlock()
		return nil
	}
	delete(r.pools, name)
	// The configured count is what was charged at creation.
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
