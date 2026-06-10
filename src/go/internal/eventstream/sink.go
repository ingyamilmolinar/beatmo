// Package eventstream persists every event published on a hooks.Bus to
// a newline-delimited JSON file (JSONL). Modeled on internal/scopeexport's
// flight-recorder pattern: env-gated, append-only, periodic flush, lossy
// under saturation (drops counted, never blocks the publisher).
//
// Writes happen on a single dedicated worker pulled from the process-wide
// async.Registry so the sink never spawns more goroutines than the
// system budget allows.
package eventstream

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/async"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// Record is the JSON shape written per line.
type Record struct {
	Seq     int64        `json:"seq"`
	At      time.Time    `json:"at"`
	MonoNs  int64        `json:"mono_ns"`
	Kind    string       `json:"kind"`
	Source  hooks.Source `json:"src,omitempty"`
	Payload any          `json:"payload,omitempty"`
}

// Options configures a Sink.
type Options struct {
	// Verbose includes high-frequency Verbose* events (camera pan, zoom,
	// drag-progress). Default: false (filtered out).
	Verbose bool
	// FlushInterval is how often the buffered writer is flushed to disk.
	// Default: 250 ms.
	FlushInterval time.Duration
	// QueueSize is the per-sink event-queue depth. Events queued past
	// this depth are dropped (counted via Dropped()). Default: 1024.
	QueueSize int
	// MaxBytes triggers file rotation (file → file.1 → file.2 …) when
	// exceeded. 0 = unlimited.
	MaxBytes int64
}

// Sink subscribes to all hooks.Kinds on a bus and writes a JSONL trace
// to the configured file. Construct via Open; Close drains and flushes.
type Sink struct {
	pool      *async.Pool
	file      *os.File
	bufW      *bufio.Writer
	enc       *json.Encoder
	queue     chan Record
	wg        sync.WaitGroup
	flushTick *time.Ticker
	startMono int64

	seq      atomic.Int64
	written  atomic.Int64
	dropped  atomic.Int64
	bytesOut atomic.Int64

	closed atomic.Bool
	stopCh chan struct{}

	unsubs []func()
	opts   Options
	path   string
}

// Open creates a Sink that writes to path and subscribes to every Kind
// in hooks.KindAll on bus. Returns a no-op Sink (with a nil queue) when
// path is empty so callers can unconditionally Open + Close.
func Open(bus *hooks.Bus, path string, opts Options) (*Sink, error) {
	if path == "" {
		return &Sink{}, nil // no-op sink
	}
	if bus == nil {
		return nil, fmt.Errorf("eventstream: nil bus")
	}
	if opts.FlushInterval <= 0 {
		opts.FlushInterval = 250 * time.Millisecond
	}
	if opts.QueueSize <= 0 {
		opts.QueueSize = 1024
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("eventstream: open %s: %w", path, err)
	}

	s := &Sink{
		file:      f,
		bufW:      bufio.NewWriterSize(f, 16*1024),
		queue:     make(chan Record, opts.QueueSize),
		flushTick: time.NewTicker(opts.FlushInterval),
		startMono: monotonicNow(),
		stopCh:    make(chan struct{}),
		opts:      opts,
		path:      path,
	}
	s.enc = json.NewEncoder(s.bufW)

	pool, err := async.DefaultRegistry().Get("eventstream.persist", async.Options{
		MaxConcurrent: 1,
		QueueSize:     8,
		Name:          "eventstream.persist",
	})
	if err != nil {
		s.close()
		return nil, fmt.Errorf("eventstream: registry pool: %w", err)
	}
	s.pool = pool

	// Spawn the writer goroutine via the pool. We use one long-running
	// task here instead of one task per event; this keeps the pool's
	// queue free for other consumers and avoids per-event submit cost.
	s.wg.Add(1)
	if err := s.pool.SubmitBlocking(context.Background(), func(_ context.Context) {
		defer s.wg.Done()
		s.run()
	}); err != nil {
		s.close()
		return nil, fmt.Errorf("eventstream: submit writer: %w", err)
	}

	// Subscribe to every Kind. Subscribers run on the bus's fan-out pool;
	// they do not write to disk — they enqueue to s.queue (non-blocking).
	for _, k := range hooks.KindAll {
		if !opts.Verbose && hooks.IsVerbose(k) {
			continue
		}
		kind := k
		unsub := bus.Subscribe(kind, func(e hooks.Event) {
			s.enqueue(e)
		})
		s.unsubs = append(s.unsubs, unsub)
	}

	return s, nil
}

func (s *Sink) enqueue(e hooks.Event) {
	if s == nil || s.queue == nil || s.closed.Load() {
		return
	}
	// Note: Seq is assigned in writeOne when the record is actually
	// written, NOT here. Subscriber callbacks run concurrently on the
	// bus's fan-out pool — assigning Seq here would interleave under
	// contention because (atomic.Add, channel send) is not atomic as a
	// pair. Assign-on-write guarantees Seq matches write order.
	rec := Record{
		At:      e.At,
		MonoNs:  monotonicNow() - s.startMono,
		Kind:    string(e.Kind),
		Source:  e.Source,
		Payload: e.Payload,
	}
	if rec.At.IsZero() {
		rec.At = time.Now()
	}
	select {
	case s.queue <- rec:
	default:
		s.dropped.Add(1)
	}
}

func (s *Sink) run() {
	for {
		select {
		case rec, ok := <-s.queue:
			if !ok {
				s.flush()
				return
			}
			s.writeOne(rec)
		case <-s.flushTick.C:
			s.flush()
			s.maybeRotate()
		case <-s.stopCh:
			// Drain any remaining queued events before exiting.
			for {
				select {
				case rec := <-s.queue:
					s.writeOne(rec)
				default:
					s.flush()
					return
				}
			}
		}
	}
}

func (s *Sink) writeOne(rec Record) {
	rec.Seq = s.seq.Add(1)
	if err := s.enc.Encode(rec); err != nil {
		// Encoder errors are fatal for this record but not the sink.
		// We track them implicitly — the next successful flush will
		// resume normal operation.
		_ = err
		return
	}
	s.written.Add(1)
}

func (s *Sink) flush() {
	if s.bufW == nil {
		return
	}
	before := s.bufW.Buffered()
	if err := s.bufW.Flush(); err == nil {
		s.bytesOut.Add(int64(before))
	}
}

func (s *Sink) maybeRotate() {
	if s.opts.MaxBytes <= 0 || s.file == nil {
		return
	}
	info, err := s.file.Stat()
	if err != nil || info.Size() < s.opts.MaxBytes {
		return
	}
	// Rotate: file → file.1 (overwriting any existing .1).
	rotated := s.path + ".1"
	_ = s.bufW.Flush()
	_ = s.file.Close()
	_ = os.Rename(s.path, rotated)
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	s.file = f
	s.bufW.Reset(f)
}

// Stats returns counters for monitoring / perfStats.
type Stats struct {
	Written  int64
	Dropped  int64
	BytesOut int64
	Path     string
	Active   bool
}

// Stats returns a snapshot of sink counters.
func (s *Sink) Stats() Stats {
	if s == nil {
		return Stats{}
	}
	return Stats{
		Written:  s.written.Load(),
		Dropped:  s.dropped.Load(),
		BytesOut: s.bytesOut.Load(),
		Path:     s.path,
		Active:   s.queue != nil && !s.closed.Load(),
	}
}

// Close drains the queue, flushes the buffered writer, unsubscribes
// from the bus, and closes the file. Safe to call multiple times.
// Safe on a no-op sink (returned when path was empty in Open).
func (s *Sink) Close() error {
	if s == nil || s.closed.Swap(true) {
		return nil
	}
	for _, u := range s.unsubs {
		u()
	}
	if s.stopCh != nil {
		close(s.stopCh)
	}
	s.wg.Wait()
	return s.close()
}

func (s *Sink) close() error {
	if s.flushTick != nil {
		s.flushTick.Stop()
	}
	var err error
	if s.bufW != nil {
		_ = s.bufW.Flush()
	}
	if s.file != nil {
		err = s.file.Close()
	}
	return err
}

// monotonicNow returns the monotonic clock in ns. We use time.Now's
// monotonic component indirectly: the difference between two
// time.Now().UnixNano() calls is wall-clock; we want monotonic. Go's
// runtime exposes monotonic via time.Since on a reference time.Time,
// so we wrap that.
var startWall = time.Now()

func monotonicNow() int64 {
	return int64(time.Since(startWall))
}

// FlushNow is exposed for tests that want deterministic flush before
// reading the file back.
func (s *Sink) FlushNow() {
	if s == nil {
		return
	}
	if !s.closed.Load() {
		time.Sleep(s.opts.FlushInterval + 50*time.Millisecond)
	}
	s.flush()
}

// _ unused interface — kept for forward compatibility with future
// abstraction (e.g., binary sink, S3 sink).
var _ io.Closer = (*Sink)(nil)
