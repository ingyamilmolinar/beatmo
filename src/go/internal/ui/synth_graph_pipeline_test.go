package ui

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/async"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Renders NOW and the dial's ends off the UI goroutine: latest-wins during a drag, the previous picture kept while
// pending or failed, a bounded cache.

type fakeGraphRender struct {
	mu    sync.Mutex
	calls int
	fail  bool
}

func (f *fakeGraphRender) render(id instrument.ID, d instrument.Doc, pitch float64) ([]float32, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.fail {
		return nil, 0, errors.New("render failed")
	}
	// A tone whose pitch is the document's "pitch" field: distinguishable renders.
	hz := 220 * math.Pow(2, audio.FlatFieldMap(d)["pitch"]/12)
	out := make([]float32, 4800)
	for i := range out {
		out[i] = float32(0.5 * math.Sin(2*math.Pi*hz*float64(i)/48000))
	}
	return out, 48000, nil
}

func graphReqFor(t *testing.T, pitch float64) func() graphReq {
	t.Helper()
	return func() graphReq {
		d, err := audio.InstrumentDocWithFields(roster.Violin, map[string]float64{"pitch": pitch})
		if err != nil {
			t.Fatal(err)
		}
		return graphReq{id: roster.Violin, doc: d}
	}
}

func identFor(n int) graphIdent { return graphIdent{hash: "h", variant: "now", n: n} }

func TestGraphPipelineRendersTheWantedDocument(t *testing.T) {
	f := &fakeGraphRender{}
	p := newSynthGraphPipelineForTest(f.render)
	p.want(slotNow, identFor(1), graphReqFor(t, 0))
	if _, ok := p.get(slotNow); ok {
		t.Fatal("a render landed before the job ran")
	}
	if !p.pending(slotNow) {
		t.Fatal("a wanted slot must report pending")
	}
	p.drainForTest()
	r, ok := p.get(slotNow)
	if !ok || r.an == nil || r.sr != 48000 {
		t.Fatalf("no analysed render landed: %+v %v", r, ok)
	}
	if p.pending(slotNow) {
		t.Fatal("still pending after the render landed")
	}
	if !p.takeDirty() || p.takeDirty() {
		t.Fatal("a landing must raise the dirty flag exactly once")
	}
}

func TestGraphPipelineCoalescesADrag(t *testing.T) {
	f := &fakeGraphRender{}
	p := newSynthGraphPipelineForTest(f.render)
	for i := 1; i <= 10; i++ {
		p.want(slotNow, identFor(i), graphReqFor(t, float64(i)))
	}
	p.drainForTest()
	if f.calls > 2 {
		t.Fatalf("ten requests during a drag ran %d renders, want at most 2 (latest wins)", f.calls)
	}
	r, _ := p.get(slotNow)
	if r == nil || r.ident != identFor(10) {
		t.Fatalf("the landed render is %+v, want the LATEST request", r)
	}
}

func TestGraphPipelineKeepsTheLastPictureWhilePendingOrFailed(t *testing.T) {
	f := &fakeGraphRender{}
	p := newSynthGraphPipelineForTest(f.render)
	p.want(slotNow, identFor(1), graphReqFor(t, 0))
	p.drainForTest()
	first, _ := p.get(slotNow)
	p.want(slotNow, identFor(2), graphReqFor(t, 5))
	if r, ok := p.get(slotNow); !ok || r != first {
		t.Fatal("a pending render must keep the previous picture (no blank frame)")
	}
	f.fail = true
	p.drainForTest()
	if r, ok := p.get(slotNow); !ok || r != first {
		t.Fatal("a failed render must keep the previous picture")
	}
	if p.pending(slotNow) {
		t.Fatal("a failed render is no longer pending")
	}
}

func TestGraphPipelineCacheIsBounded(t *testing.T) {
	f := &fakeGraphRender{}
	p := newSynthGraphPipelineForTest(f.render)
	for i := 0; i < 20; i++ {
		p.want(slotNow, identFor(i), graphReqFor(t, float64(i)))
		p.drainForTest()
	}
	if n := p.cacheLen(); n > graphCacheCap {
		t.Fatalf("cache holds %d renders, cap %d", n, graphCacheCap)
	}
	// A recent document comes back from the cache: no new render.
	calls := f.calls
	p.want(slotLo, identFor(19), graphReqFor(t, 19))
	if r, ok := p.get(slotLo); !ok || r.ident != identFor(19) {
		t.Fatal("a cached document must land at once")
	}
	if f.calls != calls {
		t.Fatalf("a cache hit rendered again (%d -> %d calls)", calls, f.calls)
	}
}

// Race-free under go test -race.
func TestGraphPipelineOnAPoolLandsTheLatest(t *testing.T) {
	f := &fakeGraphRender{}
	pool := async.NewPool(context.Background(), async.Options{MaxConcurrent: 1, QueueSize: 8, Name: "test.graphpipe"})
	t.Cleanup(func() { _ = pool.Close() })
	p := newSynthGraphPipelineForTest(f.render)
	p.pool = pool
	for i := 1; i <= 30; i++ {
		p.want(slotNow, identFor(i), graphReqFor(t, float64(i%12)))
		_, _ = p.get(slotNow)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if r, ok := p.get(slotNow); ok && r.ident == identFor(30) {
			return
		}
		p.pump() // a request the full queue refused is resubmitted
		time.Sleep(5 * time.Millisecond)
	}
	r, _ := p.get(slotNow)
	t.Fatalf("the latest request never landed: %+v", r)
}

// synth.preview is shared, so a full queue refuses; pump resubmits every frame.
func TestGraphPipelineResubmitsARefusedJob(t *testing.T) {
	f := &fakeGraphRender{}
	pool := async.NewPool(context.Background(), async.Options{MaxConcurrent: 1, QueueSize: 1, Name: "test.graphpipe.full"})
	t.Cleanup(func() { _ = pool.Close() })
	block := make(chan struct{})
	_ = pool.Submit(func(ctx context.Context) { <-block }) // occupy the worker
	_ = pool.Submit(func(ctx context.Context) {})          // fill the queue
	p := newSynthGraphPipelineForTest(f.render)
	p.pool = pool
	p.want(slotNow, identFor(1), graphReqFor(t, 0)) // refused: queue full
	close(block)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		p.pump()
		if _, ok := p.get(slotNow); ok {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("a job the full queue refused never ran")
}

// Over 300 ms it says so; a quick one never flashes the chip.
func TestGraphPipelineReportsASlowRender(t *testing.T) {
	f := &fakeGraphRender{}
	p := newSynthGraphPipelineForTest(f.render)
	start := time.Unix(9000, 0)
	now := start
	p.now = func() time.Time { return now }
	p.want(slotNow, identFor(1), graphReqFor(t, 0))
	if p.slow(slotNow) {
		t.Fatal("a render that was just asked for is not slow yet")
	}
	now = start.Add(350 * time.Millisecond)
	if !p.slow(slotNow) {
		t.Fatal("a render pending for 350 ms must report slow")
	}
	p.drainForTest()
	if p.slow(slotNow) {
		t.Fatal("a landed render is not slow")
	}
}
