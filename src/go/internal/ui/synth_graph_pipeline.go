package ui

// synth_graph_pipeline.go — renders the documents the Synth tab's graph shows
// and analyses them (spec docs/superpowers/specs/2026-10-24-synth-graphs-and-devtools-design.md §4.3).
//
// Slots: NOW (the live document), LO / HI (the focused control at its two
// ends). Each slot holds its LANDED render (stale-while-revalidate: the
// previous picture keeps drawing until the next lands, and after a failure)
// and at most one pending job; a new request replaces the pending one, so a
// drag that streams a request per detent renders only the latest (one job in
// flight plus the newest). Renders run on synth.preview — the 1-worker pool
// the mirror owns (no new registry name) — through audio.RenderDocForGraph,
// the exact voice a trigger plays. The analysis (soundview) is warmed on the
// worker before the render is handed over, so the UI goroutine only reads.

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/async"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/soundview"
)

type graphSlot int

const (
	slotNow graphSlot = iota // the live document
	slotLo                   // the focused control at its low end
	slotHi                   // the focused control at its high end
	graphSlotCount
)

// graphCacheCap bounds the cache (a 2 s render is ~380 KB; 16 ≈ 6 MB).
const graphCacheCap = 16

// graphIdent is the cheap, comparable identity of a slot's request: the
// document identity (synthParamsHash: id + revision), the Sampler edit (it is applied to the render but does not bump the revision), and what
// the slot varies. Equal idents are equal audio, so a frame that asks again
// costs one struct compare.
type graphIdent struct {
	inst    instrument.ID // whose sound: a render is never shown for another
	hash    string
	edit    audio.SampleEdit
	variant string // "now", "lo", "hi"
	ctl     string // the control the variant moves
	n       int    // extra discriminator
	// key is the CONTENT of an end ghost's document (audio.GraphRenderKey):
	// the ends do not change while their own control turns, so they are
	// identified by what they sound like, not by the live revision.
	key uint64
}

// graphReq is a render request: a document to render as instrument id.
type graphReq struct {
	id   instrument.ID
	doc  instrument.Doc
	need soundview.Need // the analyses to warm on the worker (0 = all)
}

// graphRender is a landed, analysed render.
type graphRender struct {
	ident    graphIdent
	key      uint64
	sr       int
	voiceSec float64
	an       *soundview.Analysis
}

type graphSlotState struct {
	ident     graphIdent
	landedFor graphIdent // the request landed satisfied (stale-while-revalidate)
	wanted    bool
	pending   func()
	submitted bool // a pool wrapper for pending is queued
	landed    *graphRender
	askedAt   time.Time // when the current ident was first wanted
}

type graphRenderFunc func(id instrument.ID, d instrument.Doc, pitch float64) ([]float32, int, error)

type synthGraphPipeline struct {
	mu     sync.Mutex
	pool   *async.Pool // nil = manual (tests drain)
	render graphRenderFunc
	slots  [graphSlotCount]graphSlotState
	cache  map[graphIdent]*graphRender // by cacheKey: one render per sound
	order  []graphIdent                // LRU order (cache keys), oldest first
	dirty  atomic.Bool
	busy   sync.WaitGroup   // jobs taken and still running
	now    func() time.Time // injectable clock; nil = time.Now
	// warming holds the analyses still to compute for landed renders, so a
	// view switch on the same render warms on the worker instead of the UI
	// goroutine (warm / takeWarm).
	warming map[*graphRender]soundview.Need
	warmSub map[*graphRender]bool

	// Dial relevance (synth_dial_relevance.go): sounds waiting to be measured,
	// ONE probe per pool job so a graph render queued meanwhile never waits
	// for a whole instrument; the verdicts by sound signature.
	relQueue []*relevanceJob
	relSub   bool
	relDone  map[uint64][]string
	relCount int
	// relStep serialises relevance steps: the pool's worker and a synchronous
	// drain (scene setup, tests) may both step the queue.
	relStep sync.Mutex
}

// relevanceJob is one sound's dial measurement in progress.
type relevanceJob struct {
	sig    uint64
	id     instrument.ID
	doc    instrument.Doc
	probes []dialProbe
	next   int
	hidden []string
	failed bool
}

// graphSlowAfter is how long a render may be pending before the graph says
// it is updating (the previous picture keeps drawing meanwhile).
const graphSlowAfter = 300 * time.Millisecond

func (p *synthGraphPipeline) clock() time.Time {
	if p.now != nil {
		return p.now()
	}
	return time.Now()
}

// slow reports whether slot has waited more than graphSlowAfter for the
// render of what it wants.
func (p *synthGraphPipeline) slow(slot graphSlot) bool {
	p.mu.Lock()
	st := p.slots[slot]
	p.mu.Unlock()
	if !st.wanted || (st.landed != nil && st.landedFor == st.ident) {
		return false
	}
	return p.clock().Sub(st.askedAt) > graphSlowAfter
}

// graphAnalysisYield runs between analysis stages on the worker. The browser
// build sets it to hand the single thread back to the frame loop.
var graphAnalysisYield func()

func newSynthGraphPipeline(pool *async.Pool) *synthGraphPipeline {
	return &synthGraphPipeline{pool: pool, render: audio.RenderDocForGraph, cache: map[graphIdent]*graphRender{}}
}

// newSynthGraphPipelineForTest builds a pipeline whose jobs run only when the
// test drains them (drainForTest), with render as the renderer.
func newSynthGraphPipelineForTest(render graphRenderFunc) *synthGraphPipeline {
	return &synthGraphPipeline{render: render, cache: map[graphIdent]*graphRender{}}
}

// want asks slot to show the document mk builds, identified by ident. A
// repeated ident is a no-op; a cached one lands at once; otherwise a render is
// scheduled (replacing any pending one for the slot).
func (p *synthGraphPipeline) want(slot graphSlot, ident graphIdent, mk func() graphReq) {
	p.mu.Lock()
	st := &p.slots[slot]
	if st.wanted && st.ident == ident {
		p.mu.Unlock()
		return
	}
	st.ident, st.wanted, st.askedAt = ident, true, p.clock()
	if r, ok := p.cache[cacheKey(ident)]; ok {
		st.landed, st.landedFor, st.pending = r, ident, nil
		p.touchLocked(cacheKey(ident))
		p.mu.Unlock()
		p.dirty.Store(true)
		return
	}
	req := mk()
	job := func() { p.run(slot, ident, req) }
	st.pending = job
	resubmit := !st.submitted
	p.mu.Unlock()
	if resubmit {
		p.submit(slot)
	}
}

// submit queues a wrapper that runs slot's pending job once. A refused
// submission (synth.preview's queue is shared and bounded) leaves submitted
// false, and pump retries it on a later frame.
func (p *synthGraphPipeline) submit(slot graphSlot) {
	if p.pool == nil {
		return
	}
	p.mu.Lock()
	p.slots[slot].submitted = true
	p.mu.Unlock()
	if err := p.pool.Submit(func(ctx context.Context) { p.takePending(slot) }); err != nil {
		p.mu.Lock()
		p.slots[slot].submitted = false
		p.mu.Unlock()
	}
}

// pump resubmits every pending job whose submission was refused. Called once
// per frame by the graph's draw.
func (p *synthGraphPipeline) pump() {
	for s := graphSlot(0); s < graphSlotCount; s++ {
		p.mu.Lock()
		need := p.slots[s].pending != nil && !p.slots[s].submitted
		p.mu.Unlock()
		if need {
			p.submit(s)
		}
	}
	if p.pool == nil {
		return
	}
	p.mu.Lock()
	var refused []*graphRender
	for r := range p.warming {
		if !p.warmSub[r] {
			p.warmSub[r] = true
			refused = append(refused, r)
		}
	}
	p.mu.Unlock()
	for _, r := range refused {
		r := r
		if err := p.pool.Submit(func(ctx context.Context) { p.takeWarm(r) }); err != nil {
			p.mu.Lock()
			p.warmSub[r] = false
			p.mu.Unlock()
		}
	}
	p.submitRelevance()
}

// measureRelevance queues the measurement of a sound's dials (no-op when it
// is known or already queued).
func (p *synthGraphPipeline) measureRelevance(sig uint64, id instrument.ID, doc instrument.Doc, probes []dialProbe) {
	p.mu.Lock()
	if _, done := p.relDone[sig]; done {
		p.mu.Unlock()
		return
	}
	for _, j := range p.relQueue {
		if j.sig == sig {
			p.mu.Unlock()
			return
		}
	}
	p.relQueue = append(p.relQueue, &relevanceJob{sig: sig, id: id, doc: doc, probes: probes})
	p.mu.Unlock()
	p.submitRelevance()
}

// submitRelevance queues one probe on the worker (a refused submission is
// retried by pump).
func (p *synthGraphPipeline) submitRelevance() {
	if p.pool == nil {
		return
	}
	p.mu.Lock()
	if p.relSub || len(p.relQueue) == 0 {
		p.mu.Unlock()
		return
	}
	p.relSub = true
	p.mu.Unlock()
	if err := p.pool.Submit(func(ctx context.Context) { p.relevanceStep() }); err != nil {
		p.mu.Lock()
		p.relSub = false
		p.mu.Unlock()
	}
}

// relevanceStep measures the next probe of the oldest queued sound; the last
// probe lands the verdict. Then the next step is queued behind whatever the
// graph asked for meanwhile. Steps never overlap (relStep), so each probe
// lands once and each sound is finished once.
func (p *synthGraphPipeline) relevanceStep() {
	p.relStep.Lock()
	defer p.relStep.Unlock()
	p.mu.Lock()
	if len(p.relQueue) == 0 {
		p.relSub = false
		p.mu.Unlock()
		return
	}
	job := p.relQueue[0]
	i := job.next
	job.next++
	p.busy.Add(1)
	p.mu.Unlock()
	defer p.busy.Done()
	if i < len(job.probes) && !job.failed {
		ok, err := probeRelevant(job.id, job.doc, job.probes[i], p.render, graphAnalysisYield)
		switch {
		case err != nil:
			job.failed = true // keep every dial: never hide on a failed render
		case !ok:
			job.hidden = append(job.hidden, job.probes[i].key)
		}
	}
	p.mu.Lock()
	if (job.next >= len(job.probes) || job.failed) && len(p.relQueue) > 0 && p.relQueue[0] == job {
		if p.relDone == nil {
			p.relDone = map[uint64][]string{}
		}
		if job.failed {
			job.hidden = nil
		}
		sort.Strings(job.hidden)
		p.relDone[job.sig] = job.hidden
		p.relCount++
		p.relQueue = p.relQueue[1:]
		p.dirty.Store(true)
	}
	p.relSub = false
	p.mu.Unlock()
	p.submitRelevance()
}

// relevanceVerdict returns the measured verdict for a sound signature.
func (p *synthGraphPipeline) relevanceVerdict(sig uint64) ([]string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	h, ok := p.relDone[sig]
	return h, ok
}

// relevanceMeasured is how many sounds have been measured (tests).
func (p *synthGraphPipeline) relevanceMeasured() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.relCount
}

// relevancePending reports whether a measurement is queued or running.
func (p *synthGraphPipeline) relevancePending() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.relQueue) > 0
}

// clear forgets what slot wants (its landed render is dropped too).
func (p *synthGraphPipeline) clear(slot graphSlot) {
	p.mu.Lock()
	p.slots[slot] = graphSlotState{}
	p.mu.Unlock()
}

func (p *synthGraphPipeline) takePending(slot graphSlot) {
	p.mu.Lock()
	job := p.slots[slot].pending
	p.slots[slot].pending = nil
	p.slots[slot].submitted = false
	if job != nil {
		p.busy.Add(1)
	}
	p.mu.Unlock()
	if job != nil {
		defer p.busy.Done()
		job()
	}
}

// waitIdle blocks until no job is running (scene Setup settles on it).
func (p *synthGraphPipeline) waitIdle() { p.busy.Wait() }

// run renders + analyses req and lands it on slot (when slot still wants it).
func (p *synthGraphPipeline) run(slot graphSlot, ident graphIdent, req graphReq) {
	// Another slot may have rendered this very sound while this job waited.
	p.mu.Lock()
	if r, ok := p.cache[cacheKey(ident)]; ok {
		p.landLocked(slot, ident, r)
		p.mu.Unlock()
		p.dirty.Store(true)
		return
	}
	p.mu.Unlock()
	pcm, sr, err := p.render(req.id, req.doc, 0)
	if err != nil || len(pcm) == 0 {
		return // keep the previous picture
	}
	voice := audio.GraphVoiceSec(req.doc)
	an := soundview.New(pcm, sr, voice)
	need := req.need
	if need == 0 {
		need = soundview.NeedAll
	}
	an.Warm(need, graphAnalysisYield)
	r := &graphRender{ident: ident, key: audio.GraphRenderKey(req.id, req.doc, 0), sr: sr, voiceSec: voice, an: an}
	p.mu.Lock()
	p.cache[cacheKey(ident)] = r
	p.touchLocked(cacheKey(ident))
	p.landLocked(slot, ident, r)
	p.mu.Unlock()
	p.dirty.Store(true)
}

// landLocked lands r on slot (when it still wants ident) and on every other
// slot waiting for the same sound.
func (p *synthGraphPipeline) landLocked(slot graphSlot, ident graphIdent, r *graphRender) {
	for s := range p.slots {
		st := &p.slots[s]
		if !st.wanted || (graphSlot(s) != slot && cacheKey(st.ident) != cacheKey(ident)) || (graphSlot(s) == slot && st.ident != ident) {
			continue
		}
		st.landed, st.landedFor = r, st.ident
		if graphSlot(s) != slot {
			st.pending = nil
		}
	}
}

// cacheKey is what a render is cached under: the sound itself when the
// request names its content (audio.GraphRenderKey: the document's sound
// signature, the tempo, the pitch, the Sampler edit), so every slot and every
// revision that asks for the same sound shares one render.
func cacheKey(id graphIdent) graphIdent {
	if id.key != 0 {
		return graphIdent{inst: id.inst, key: id.key}
	}
	return id
}

func (p *synthGraphPipeline) touchLocked(ident graphIdent) {
	for i, id := range p.order {
		if id == ident {
			p.order = append(p.order[:i], p.order[i+1:]...)
			break
		}
	}
	p.order = append(p.order, ident)
	for len(p.order) > graphCacheCap {
		delete(p.cache, p.order[0])
		p.order = p.order[1:]
	}
}

// get returns slot's landed render (the previous one while a new one renders).
func (p *synthGraphPipeline) get(slot graphSlot) (*graphRender, bool) {
	r, _, ok := p.getFor(slot)
	return r, ok
}

// getFor is get plus the request the landed render satisfied for slot.
func (p *synthGraphPipeline) getFor(slot graphSlot) (*graphRender, graphIdent, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	st := p.slots[slot]
	return st.landed, st.landedFor, st.landed != nil
}

// wanted returns the ident slot currently asks for.
func (p *synthGraphPipeline) wanted(slot graphSlot) (graphIdent, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	st := p.slots[slot]
	return st.ident, st.wanted
}

// lookup returns a cached render by ident (it may have landed on no slot).
func (p *synthGraphPipeline) lookup(ident graphIdent) (*graphRender, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	r, ok := p.cache[cacheKey(ident)]
	return r, ok
}

// pending reports whether slot is waiting for a render of what it wants.
func (p *synthGraphPipeline) pending(slot graphSlot) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	st := p.slots[slot]
	return st.wanted && (st.landed == nil || st.landedFor != st.ident) && st.pending != nil
}

// warm asks for the analyses need names on r, on the worker; ready reports
// whether they are computed already (then nothing is scheduled).
func (p *synthGraphPipeline) warm(r *graphRender, need soundview.Need) (ready bool) {
	if r == nil || r.an == nil || r.an.Has(need) {
		return true
	}
	p.mu.Lock()
	if p.warming == nil {
		p.warming = map[*graphRender]soundview.Need{}
		p.warmSub = map[*graphRender]bool{}
	}
	p.warming[r] |= need
	sub := p.warmSub[r]
	if p.pool != nil && !sub {
		p.warmSub[r] = true
	}
	p.mu.Unlock()
	if p.pool != nil && !sub {
		if err := p.pool.Submit(func(ctx context.Context) { p.takeWarm(r) }); err != nil {
			p.mu.Lock()
			p.warmSub[r] = false
			p.mu.Unlock()
		}
	}
	return false
}

func (p *synthGraphPipeline) takeWarm(r *graphRender) {
	p.mu.Lock()
	need, ok := p.warming[r]
	delete(p.warming, r)
	delete(p.warmSub, r)
	if ok {
		p.busy.Add(1)
	}
	p.mu.Unlock()
	if !ok {
		return
	}
	defer p.busy.Done()
	r.an.Warm(need, graphAnalysisYield)
	p.dirty.Store(true)
}

// takeDirty reports (and clears) whether a render landed since the last call.
func (p *synthGraphPipeline) takeDirty() bool { return p.dirty.Swap(false) }

// renderNowSync renders slot's request on the caller goroutine (scene Setup:
// a deterministic picture at capture time).
func (p *synthGraphPipeline) renderNowSync(slot graphSlot, ident graphIdent, mk func() graphReq) {
	p.want(slot, ident, mk)
	p.takePending(slot)
}

// drainForTest runs every pending job (renders, then warms) on the caller
// goroutine.
func (p *synthGraphPipeline) drainForTest() {
	for s := graphSlot(0); s < graphSlotCount; s++ {
		p.takePending(s)
	}
	p.mu.Lock()
	var rs []*graphRender
	for r := range p.warming {
		rs = append(rs, r)
	}
	p.mu.Unlock()
	for _, r := range rs {
		p.takeWarm(r)
	}
	for p.relevancePending() {
		p.relevanceStep()
	}
}

func (p *synthGraphPipeline) cacheLen() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.cache)
}

// synthPreviewPoolName is the Synth tab's standing render pool (CLAUDE.md
// § standing pools): every graph render runs on it.
const synthPreviewPoolName = "synth.preview"

// newSynthPreviewPool acquires the shared 1-worker preview pool from the
// default registry. The pipeline is created LAZILY mid-session (the Synth tab's
// first graph), so registry-budget exhaustion must NOT crash the app: like
// eventlogger.format and hooks.fanout, fall back to a private pool when the
// shared budget is full.
func newSynthPreviewPool() *async.Pool {
	opts := async.Options{MaxConcurrent: 1, QueueSize: 8, Name: synthPreviewPoolName}
	pool, err := async.DefaultRegistry().Get(synthPreviewPoolName, opts)
	if err != nil {
		opts.Name = synthPreviewPoolName + ".fallback"
		return async.NewPool(context.Background(), opts)
	}
	return pool
}

// synthGraphPipe returns the DrumView's graph pipeline, created on first use
// on the shared preview pool.
func (dv *DrumView) synthGraphPipe() *synthGraphPipeline {
	if dv.graphPipe == nil {
		dv.graphPipe = newSynthGraphPipeline(newSynthPreviewPool())
	}
	return dv.graphPipe
}
