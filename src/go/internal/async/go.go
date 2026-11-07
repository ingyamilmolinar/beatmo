package async

import "context"

// Fire-and-forget tasks are infrequent, and one worker keeps them serialised within a name.
var goPoolDefaults = Options{MaxConcurrent: 1, QueueSize: 8}

// For one-shot, low-frequency tasks that still want bounded goroutines (file dialogs, init hand-offs); latency-sensitive
// work takes its own pool with explicit Options. The pool is created on first call per name (opts apply only then).
// Returns Submit's error, or ErrBudgetExhausted on a creation over budget.
func Go(name string, fn func(ctx context.Context)) error {
	opts := goPoolDefaults
	opts.Name = name
	p, err := DefaultRegistry().Get(name, opts)
	if err != nil {
		return err
	}
	return p.Submit(Job(fn))
}
