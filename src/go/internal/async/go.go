package async

import "context"

// goPoolDefaults is the Options used by Go to lazy-create a named pool
// in DefaultRegistry on first use. Conservative — fire-and-forget tasks
// are infrequent, and 1 worker keeps them serialized within a name.
var goPoolDefaults = Options{MaxConcurrent: 1, QueueSize: 8}

// Go submits fn to the named pool in DefaultRegistry, lazy-creating the
// pool with conservative defaults (1 worker, queue 8) on first call
// for each name. Subsequent calls reuse the existing pool — opts only
// apply on creation, per Registry.Get's contract.
//
// Use for one-shot, low-frequency tasks where a dedicated pool would be
// overkill but goroutine bounds are still wanted (file dialogs, init
// hand-offs, periodic housekeeping). For high-throughput or
// latency-sensitive work, acquire a pool from DefaultRegistry directly
// with explicit Options.
//
// Returns Pool.Submit's error (ErrBackpressure when saturated, ErrClosed
// during shutdown) or ErrBudgetExhausted if the pool's worker count
// would push the registry over Budget on first creation.
func Go(name string, fn func(ctx context.Context)) error {
	opts := goPoolDefaults
	opts.Name = name
	p, err := DefaultRegistry().Get(name, opts)
	if err != nil {
		return err
	}
	return p.Submit(Job(fn))
}
