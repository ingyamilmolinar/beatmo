package async

import (
	"container/heap"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// ErrSchedulerClosed is returned by Schedule when called after Close.
var ErrSchedulerClosed = errors.New("async: scheduler closed")

// Scheduler dispatches Jobs at deadlines onto an underlying Pool. A
// single timer goroutine owns a min-heap of pending entries; when the
// head is due, the entry is non-blocking-Submitted to the pool.
//
// Use this instead of `go func() { time.Sleep(d); fn() }` whenever the
// number of pending deadlines could grow with workload — one Scheduler
// owns one timer goroutine regardless of pending count.
//
// Drop semantics match the rest of the system: if the pool is saturated
// when an entry fires, the Scheduler counts it as Dropped and continues
// (no retries, no blocking). Callers that need backpressure visibility
// should monitor Stats().Dropped.
type Scheduler struct {
	pool *Pool

	mu     sync.Mutex
	heap   entryHeap
	closed bool
	wakeup chan struct{}
	quit   chan struct{}
	wg     sync.WaitGroup

	pending   atomic.Int64
	fired     atomic.Int64
	cancelled atomic.Int64
	dropped   atomic.Int64
}

// ScheduleStats is a point-in-time snapshot of scheduler counters.
type ScheduleStats struct {
	Pending   int64
	Fired     int64
	Cancelled int64
	Dropped   int64
}

// schedEntry state values. The entry transitions monotonically from
// statePending into exactly one terminal state, so cancel-after-fire and
// fire-after-cancel are naturally no-ops via CAS.
const (
	statePending   int32 = 0 // initial
	stateCancelled int32 = 1 // cancel() ran before fireDue claimed it
	stateConsumed  int32 = 2 // fireDue claimed the entry (fired or dropped)
)

type schedEntry struct {
	when  time.Time
	fn    Job
	state atomic.Int32 // 0 pending → 1 cancelled | 2 consumed (terminal)
	index int          // heap index, -1 once popped
}

type entryHeap []*schedEntry

func (h entryHeap) Len() int           { return len(h) }
func (h entryHeap) Less(i, j int) bool { return h[i].when.Before(h[j].when) }
func (h entryHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i]; h[i].index = i; h[j].index = j }
func (h *entryHeap) Push(x any)        { e := x.(*schedEntry); e.index = len(*h); *h = append(*h, e) }
func (h *entryHeap) Pop() any {
	old := *h
	n := len(old)
	e := old[n-1]
	old[n-1] = nil
	e.index = -1
	*h = old[:n-1]
	return e
}

// NewScheduler returns a Scheduler that fires Jobs onto pool. The
// scheduler owns one background goroutine; call Close to stop it.
func NewScheduler(pool *Pool) *Scheduler {
	if pool == nil {
		panic("async: NewScheduler requires a non-nil pool")
	}
	s := &Scheduler{
		pool:   pool,
		wakeup: make(chan struct{}, 1),
		quit:   make(chan struct{}),
	}
	s.wg.Add(1)
	go s.run()
	return s
}

// Schedule enqueues fn to run at when. The returned cancel func marks
// the entry skipped; calling it after the entry has fired is a no-op.
// Returns ErrSchedulerClosed if Close has been called.
func (s *Scheduler) Schedule(when time.Time, fn Job) (cancel func(), err error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return func() {}, ErrSchedulerClosed
	}
	e := &schedEntry{when: when, fn: fn}
	heap.Push(&s.heap, e)
	headIsNew := s.heap[0] == e
	s.mu.Unlock()
	s.pending.Add(1)

	if headIsNew {
		s.signal()
	}

	return func() {
		if e.state.CompareAndSwap(statePending, stateCancelled) {
			s.cancelled.Add(1)
		}
	}, nil
}

// Close stops the timer goroutine and drops any still-pending entries.
// Subsequent Schedule calls return ErrSchedulerClosed. Safe to call
// multiple times.
func (s *Scheduler) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	dropped := int64(s.heap.Len())
	s.heap = nil
	s.mu.Unlock()
	s.dropped.Add(dropped)
	s.pending.Add(-dropped)
	close(s.quit)
	s.wg.Wait()
	return nil
}

// Stats returns a snapshot of scheduler counters.
func (s *Scheduler) Stats() ScheduleStats {
	return ScheduleStats{
		Pending:   s.pending.Load(),
		Fired:     s.fired.Load(),
		Cancelled: s.cancelled.Load(),
		Dropped:   s.dropped.Load(),
	}
}

func (s *Scheduler) signal() {
	select {
	case s.wakeup <- struct{}{}:
	default:
	}
}

// run is the timer goroutine. It sleeps until either the head entry is
// due, a wakeup arrives (new earlier head), or the scheduler is closed.
func (s *Scheduler) run() {
	defer s.wg.Done()
	// idleSleep is long enough that a paused (no-pending) scheduler
	// doesn't spin, but short enough that a missed wakeup recovers
	// within human-scale latency. wakeup makes this rare.
	const idleSleep = time.Hour

	for {
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			return
		}
		var d time.Duration
		if s.heap.Len() == 0 {
			d = idleSleep
		} else {
			d = max(time.Until(s.heap[0].when), 0)
		}
		s.mu.Unlock()

		timer := time.NewTimer(d)
		select {
		case <-timer.C:
			s.fireDue()
		case <-s.wakeup:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		case <-s.quit:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		}
	}
}

// fireDue pops every entry whose deadline is <= now and submits the
// non-cancelled ones to the pool.
func (s *Scheduler) fireDue() {
	now := time.Now()
	var due []*schedEntry
	s.mu.Lock()
	for s.heap.Len() > 0 && !s.heap[0].when.After(now) {
		e := heap.Pop(&s.heap).(*schedEntry)
		due = append(due, e)
	}
	s.mu.Unlock()

	for _, e := range due {
		s.pending.Add(-1)
		// Claim the entry. If cancel() already won the CAS, skip — the
		// cancelled counter was bumped there.
		if !e.state.CompareAndSwap(statePending, stateConsumed) {
			continue
		}
		if err := s.pool.Submit(e.fn); err != nil {
			s.dropped.Add(1)
			continue
		}
		s.fired.Add(1)
	}
}
