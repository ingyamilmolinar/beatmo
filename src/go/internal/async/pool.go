// Package async provides a small, resource-constrained worker pool for
// running background jobs without leaking goroutines or unbounded memory.
//
// The Pool is intended for callers that must hand off work from a real-time
// thread (audio callback, UI Update) without ever blocking and without
// allocating. Submit is non-blocking: when the queue is full, jobs are
// rejected with ErrBackpressure and a drop counter is incremented. Callers
// that can wait should use SubmitBlocking, which respects context cancellation.
package async

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
)

// ErrBackpressure is returned from Submit when the pool's queue is full.
var ErrBackpressure = errors.New("async: queue full")

// ErrClosed is returned when submitting to a closed pool.
var ErrClosed = errors.New("async: pool closed")

// Job is the unit of work executed by Pool workers. It receives the pool's
// context, which is cancelled on Close.
type Job func(ctx context.Context)

// Options configures a Pool. Zero values pick reasonable defaults.
type Options struct {
	// MaxConcurrent caps the number of worker goroutines. 0 picks
	// max(1, runtime.NumCPU()/2).
	MaxConcurrent int
	// QueueSize is the bounded buffer size between Submit and workers.
	// 0 picks 64. Submit returns ErrBackpressure when full.
	QueueSize int
	// Name is used in metrics and logs.
	Name string
	// OnDrop is invoked synchronously by Submit when a job is rejected.
	// Keep it cheap — it runs on the caller's thread.
	OnDrop func(name string)
}

// Stats is a point-in-time snapshot of pool counters.
type Stats struct {
	Name      string
	Queued    int64 // jobs currently buffered, waiting for a worker
	Inflight  int64 // jobs currently executing
	Completed int64 // jobs that finished successfully
	Dropped   int64 // jobs rejected by Submit due to backpressure
	Workers   int   // configured worker count
	QueueSize int   // configured queue capacity
}

// Pool runs Jobs with bounded concurrency and a bounded queue.
type Pool struct {
	name      string
	queue     chan Job
	wg        sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc
	onDrop    func(string)
	workers   int
	queueSize int

	queued    atomic.Int64
	inflight  atomic.Int64
	completed atomic.Int64
	dropped   atomic.Int64

	closeOnce sync.Once
	closed    atomic.Bool
}

// NewPool creates and starts a Pool. The pool runs until Close is called or
// the parent context is cancelled.
func NewPool(parent context.Context, opts Options) *Pool {
	if parent == nil {
		parent = context.Background()
	}
	workers := opts.MaxConcurrent
	if workers <= 0 {
		workers = max(1, runtime.NumCPU()/2)
	}
	queueSize := opts.QueueSize
	if queueSize <= 0 {
		queueSize = 64
	}
	ctx, cancel := context.WithCancel(parent)
	p := &Pool{
		name:      opts.Name,
		queue:     make(chan Job, queueSize),
		ctx:       ctx,
		cancel:    cancel,
		onDrop:    opts.OnDrop,
		workers:   workers,
		queueSize: queueSize,
	}
	p.wg.Add(workers)
	for i := 0; i < workers; i++ {
		go p.run()
	}
	return p
}

func (p *Pool) run() {
	defer p.wg.Done()
	// Drain semantics: keep ranging until the queue is closed AND empty.
	// Close() closes the queue, so all queued jobs run before workers exit.
	// Job authors that want to abort early should respect ctx (passed to
	// each job and cancelled by Close).
	for job := range p.queue {
		p.queued.Add(-1)
		p.inflight.Add(1)
		func() {
			defer func() {
				p.inflight.Add(-1)
				p.completed.Add(1)
				// Recover so a panicking job doesn't kill the worker.
				if r := recover(); r != nil {
					_ = r
				}
			}()
			job(p.ctx)
		}()
	}
}

// Submit enqueues j without blocking. Returns ErrBackpressure if the queue
// is full or ErrClosed if the pool has been closed. Safe to call from any
// goroutine, including real-time threads.
func (p *Pool) Submit(j Job) error {
	if p.closed.Load() {
		return ErrClosed
	}
	select {
	case p.queue <- j:
		p.queued.Add(1)
		return nil
	default:
		p.dropped.Add(1)
		if p.onDrop != nil {
			p.onDrop(p.name)
		}
		return ErrBackpressure
	}
}

// SubmitBlocking enqueues j, waiting until a slot is free or ctx is done.
// Returns ctx.Err() on cancellation or ErrClosed if the pool is closed.
func (p *Pool) SubmitBlocking(ctx context.Context, j Job) error {
	if p.closed.Load() {
		return ErrClosed
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case p.queue <- j:
		p.queued.Add(1)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-p.ctx.Done():
		return ErrClosed
	}
}

// Stats returns a snapshot of pool counters.
func (p *Pool) Stats() Stats {
	return Stats{
		Name:      p.name,
		Queued:    p.queued.Load(),
		Inflight:  p.inflight.Load(),
		Completed: p.completed.Load(),
		Dropped:   p.dropped.Load(),
		Workers:   p.workers,
		QueueSize: p.queueSize,
	}
}

// Close shuts the pool down: cancels the pool context, closes the queue,
// and waits for in-flight jobs. Safe to call multiple times.
func (p *Pool) Close() error {
	p.closeOnce.Do(func() {
		p.closed.Store(true)
		p.cancel()
		close(p.queue)
	})
	p.wg.Wait()
	return nil
}
