// Package eventlogger renders one human-readable INFO line per hooks.Bus event (eventstream writes the same stream as
// JSONL). High-rate kinds go through a debounce-trailing window, so a slider drag is one line; verbose kinds are off
// unless Options.Verbose or BEATMO_EVENT_LOG_VERBOSE=1.
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

// logSeq is a process-wide monotonic sequence for retained LogRecords.
var logSeq atomic.Int64

// Options configures a Logger.
type Options struct {
	// Verbose includes hooks.IsVerbose(k) kinds (camera pan/zoom, drag
	// progress). Default: false.
	Verbose bool
	// QueueSize is the per-logger event-queue depth. Events queued past
	// this depth are dropped (counted via Dropped()). Default: 1024.
	QueueSize int
	// Debounce-trailing window for high-rate events; 0 disables coalescing.
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
	// Runs from the drain goroutine (non-coalesced events) and from the AfterFunc timers (a closed window).
	l.coal = newCoalescer(opts.CoalesceWindow, func(e hooks.Event) {
		l.writeEvent(e)
	})

	pool, err := async.DefaultRegistry().Get("eventlogger.format", async.Options{
		MaxConcurrent: 1,
		QueueSize:     8,
		Name:          "eventlogger.format",
	})
	if err != nil {
		// Budget exhausted: a private pool, so the narrative is never silently dropped.
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
		// Never "<unknown kind=X>"; coverage_test makes every non-verbose Kind have a formatter.
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

// A non-zero Source (captured at the user-action call site) is appended as "src=pkg/file.go:line" to grep to; the
// middleware (helper, bus, formatter) is never what gets logged.
func (l *Logger) writeEvent(e hooks.Event) {
	f, ok := lookupFormatter(e.Kind)
	if !ok {
		return
	}
	msg := f(e.Payload)
	if msg == "" {
		return
	}
	l.written.Add(1)

	// Compute tag + scope once: used both for the retained LogRecord and for
	// the INFO bracket tag below.
	tag := ""
	scope := hooks.ScopeSession
	if meta, ok := hooks.MetaFor(e.Kind); ok {
		tag = meta.Tag
		scope = meta.Scope
	}
	// Retain a searchable record for the UI logs panel (no-op sink by default).
	History().Record(LogRecord{
		Seq:   logSeq.Add(1),
		At:    e.At,
		Kind:  e.Kind,
		Tag:   tag,
		Scope: scope,
		Msg:   msg,
		IsErr: false,
	})

	srcSuffix := ""
	if s := e.Source.String(); s != "" {
		srcSuffix = "  src=" + s
	}
	if tag == "" {
		l.log.Infof("%s%s", msg, srcSuffix)
	} else {
		l.log.Infof("[%s] %s%s", tag, msg, srcSuffix)
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
