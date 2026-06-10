// Package eventlogger subscribes to a hooks.Bus and emits one human-readable
// INFO line per event through a *log.Logger. It is the canonical "narrative
// view" of the bus — sibling to internal/eventstream, which writes the same
// stream to a JSONL file for offline analysis.
//
// High-rate events (BPM/volume drag, EQ slider, insert-effect param tweaks,
// node drag-to-cell) are coalesced through a debounce-trailing window so a
// 50ms slider drag becomes one INFO line, not fifty. See coalesce.go.
//
// Verbose kinds (camera pan/zoom, drag progress) are filtered out by default;
// callers opt in via Options.Verbose or the BEATMO_EVENT_LOG_VERBOSE=1 env.
package eventlogger

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/async"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	gamelog "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Options configures a Logger.
type Options struct {
	// Verbose includes hooks.IsVerbose(k) kinds (camera pan/zoom, drag
	// progress). Default: false.
	Verbose bool
	// QueueSize is the per-logger event-queue depth. Events queued past
	// this depth are dropped (counted via Dropped()). Default: 1024.
	QueueSize int
	// CoalesceWindow is the debounce-trailing window for high-rate
	// events (BPM/volume drag, EQ band, insert-effect param, node move).
	// Default: 250 ms. Set to 0 to disable coalescing entirely.
	CoalesceWindow time.Duration
}

// Logger is the running consumer; construct via Open and shut down via Close.
type Logger struct {
	bus    *hooks.Bus
	log    *gamelog.Logger
	pool   *async.Pool
	queue  chan hooks.Event
	coal   *coalescer
	unsubs []func()
	wg     sync.WaitGroup
	stopCh chan struct{}
	closed atomic.Bool
	opts   Options

	enqueued atomic.Int64
	written  atomic.Int64
	dropped  atomic.Int64
}

// ErrNilBus is returned by Open when bus is nil.
var ErrNilBus = errors.New("eventlogger: nil bus")

// ErrNilLogger is returned by Open when log is nil.
var ErrNilLogger = errors.New("eventlogger: nil logger")

// Open constructs a Logger that subscribes to every Kind in hooks.KindAll
// (filtering verbose kinds when opts.Verbose is false) and emits formatted
// INFO lines through lg.
func Open(bus *hooks.Bus, lg *gamelog.Logger, opts Options) (*Logger, error) {
	if bus == nil {
		return nil, ErrNilBus
	}
	if lg == nil {
		return nil, ErrNilLogger
	}
	if opts.QueueSize <= 0 {
		opts.QueueSize = 1024
	}
	if opts.CoalesceWindow < 0 {
		opts.CoalesceWindow = 0
	} else if opts.CoalesceWindow == 0 {
		opts.CoalesceWindow = 250 * time.Millisecond
	}

	l := &Logger{
		bus:    bus,
		log:    lg,
		queue:  make(chan hooks.Event, opts.QueueSize),
		stopCh: make(chan struct{}),
		opts:   opts,
	}
	// Coalescer's emit callback is the synchronous-emit path; it runs from
	// the drain goroutine (when pulling a non-coalesced event) and from the
	// AfterFunc timer goroutine (when a coalesced window elapses).
	l.coal = newCoalescer(opts.CoalesceWindow, func(e hooks.Event) {
		l.writeEvent(e)
	})

	pool, err := async.DefaultRegistry().Get("eventlogger.format", async.Options{
		MaxConcurrent: 1,
		QueueSize:     8,
		Name:          "eventlogger.format",
	})
	if err != nil {
		// Registry budget exhausted; fall back to a private pool so the
		// logger still works rather than silently dropping the narrative.
		pool = async.NewPool(context.Background(), async.Options{
			MaxConcurrent: 1,
			QueueSize:     8,
			Name:          "eventlogger.format.fallback",
		})
	}
	l.pool = pool

	l.wg.Add(1)
	if err := l.pool.SubmitBlocking(context.Background(), func(_ context.Context) {
		defer l.wg.Done()
		l.run()
	}); err != nil {
		return nil, fmt.Errorf("eventlogger: submit drain: %w", err)
	}

	for _, k := range hooks.KindAll {
		if !opts.Verbose && hooks.IsVerbose(k) {
			continue
		}
		// Skip Kinds without a formatter so we never log "<unknown kind=X>".
		// coverage_test enforces that every non-verbose Kind has a formatter,
		// so this only filters new Kinds added without a corresponding entry.
		if _, ok := lookupFormatter(k); !ok {
			continue
		}
		kind := k
		unsub := bus.Subscribe(kind, func(e hooks.Event) {
			l.enqueue(e)
		})
		l.unsubs = append(l.unsubs, unsub)
	}

	return l, nil
}

func (l *Logger) enqueue(e hooks.Event) {
	if l == nil || l.queue == nil || l.closed.Load() {
		return
	}
	l.enqueued.Add(1)
	select {
	case l.queue <- e:
	default:
		l.dropped.Add(1)
	}
}

func (l *Logger) run() {
	for {
		select {
		case e, ok := <-l.queue:
			if !ok {
				return
			}
			l.dispatch(e)
		case <-l.stopCh:
			// Drain remaining events, then flush any coalesced-pending lines.
			for {
				select {
				case e := <-l.queue:
					l.dispatch(e)
				default:
					l.coal.FlushAll()
					return
				}
			}
		}
	}
}

// dispatch routes an event through the coalescer (which may delay it) or
// directly to writeEvent if the kind isn't coalesced.
func (l *Logger) dispatch(e hooks.Event) {
	if l.coal.IsCoalesced(e.Kind) {
		l.coal.Submit(e)
		return
	}
	l.writeEvent(e)
}

// writeEvent runs the per-Kind formatter and emits one INFO line. When the
// event carries a non-zero Source (captured at the user-action call site by
// the emit helper), it is appended as a trailing "src=pkg/file.go:line"
// column so the reader can grep directly to the source. The middleware
// layer (helper, bus, this formatter) is intentionally NOT what gets logged.
func (l *Logger) writeEvent(e hooks.Event) {
	f, ok := lookupFormatter(e.Kind)
	if !ok {
		return
	}
	out := f(e.Payload)
	if out.msg == "" {
		return
	}
	l.written.Add(1)
	srcSuffix := ""
	if s := e.Source.String(); s != "" {
		srcSuffix = "  src=" + s
	}
	if out.tag == "" {
		l.log.Infof("%s%s", out.msg, srcSuffix)
	} else {
		l.log.Infof("[%s] %s%s", out.tag, out.msg, srcSuffix)
	}
}

// Stats returns counters for monitoring / perfStats.
type Stats struct {
	Enqueued int64
	Written  int64
	Dropped  int64
	Active   bool
}

// Stats returns a snapshot of logger counters.
func (l *Logger) Stats() Stats {
	if l == nil {
		return Stats{}
	}
	return Stats{
		Enqueued: l.enqueued.Load(),
		Written:  l.written.Load(),
		Dropped:  l.dropped.Load(),
		Active:   l.queue != nil && !l.closed.Load(),
	}
}

// Close drains the queue, unsubscribes from the bus, and flushes any
// pending coalesced lines. Safe to call multiple times.
func (l *Logger) Close() error {
	if l == nil || l.closed.Swap(true) {
		return nil
	}
	for _, u := range l.unsubs {
		u()
	}
	if l.stopCh != nil {
		close(l.stopCh)
	}
	l.wg.Wait()
	return nil
}

// FlushNow blocks until the coalescer has emitted all pending lines. Tests
// use this to read deterministic output without waiting on real timers.
func (l *Logger) FlushNow() {
	if l == nil {
		return
	}
	l.coal.FlushAll()
}
