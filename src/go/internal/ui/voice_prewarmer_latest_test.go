package ui

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/async"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

func newLatestTestPrewarmer(t *testing.T, queue int) (*voicePrewarmer, func(), *[]string, *sync.Mutex) {
	t.Helper()
	pool := async.NewPool(nil, async.Options{MaxConcurrent: 1, QueueSize: queue, Name: "test.prewarm.latest"})
	t.Cleanup(func() { pool.Close() })
	gate := make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(gate) }) }
	t.Cleanup(release) // before Close (LIFO), or a failed assertion hangs Close
	started := make(chan struct{})
	if err := pool.Submit(func(context.Context) { close(started); <-gate }); err != nil {
		t.Fatal(err)
	}
	<-started // the worker is busy: everything below queues
	var mu sync.Mutex
	var rendered []string
	p := &voicePrewarmer{pool: pool, bindings: make(map[warmKey]*bindingWarm)}
	p.renderDocFn = func(id instrument.ID, pitch float64, d instrument.Doc) {
		mu.Lock()
		rendered = append(rendered, d.Name)
		mu.Unlock()
	}
	p.warmParamsFn = p.warmLatest
	return p, release, &rendered, &mu
}

func waitPoolIdle(t *testing.T, pool *async.Pool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if s := pool.Stats(); s.Queued == 0 && s.Inflight == 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("pool never went idle: %+v", pool.Stats())
}

func TestBindingWarmRendersOnlyTheNewestDocument(t *testing.T) {
	p, release, rendered, mu := newLatestTestPrewarmer(t, 64)
	for _, name := range []string{"d1", "d2", "d3", "final"} {
		p.warmLatest("piano", 0, instrument.Doc{Name: name})
	}
	if q := p.pool.Stats().Queued; q != 1 {
		t.Fatalf("%d jobs queued for one (instrument, pitch), want 1", q)
	}
	release()
	waitPoolIdle(t, p.pool)
	mu.Lock()
	defer mu.Unlock()
	if len(*rendered) != 1 || (*rendered)[0] != "final" {
		t.Fatalf("rendered %v, want only [final]", *rendered)
	}
}

func TestBindingWarmRefusedOnBackpressureIsRetried(t *testing.T) {
	p, release, rendered, mu := newLatestTestPrewarmer(t, 1)
	if err := p.pool.Submit(p.thenRetry(func() {})); err != nil { // fills the queue
		t.Fatal(err)
	}
	p.warmLatest("piano", 0, instrument.Doc{Name: "final"})
	if d := p.pool.Stats().Dropped; d != 1 {
		t.Fatalf("expected the warm to be refused (dropped=%d)", d)
	}
	release()
	waitPoolIdle(t, p.pool)
	mu.Lock()
	defer mu.Unlock()
	if len(*rendered) != 1 || (*rendered)[0] != "final" {
		t.Fatalf("rendered %v after the queue drained, want [final]", *rendered)
	}
	if len(p.bindings) != 0 {
		t.Fatalf("%d binding slots left behind", len(p.bindings))
	}
}
