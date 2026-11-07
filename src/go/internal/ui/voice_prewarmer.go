package ui

import (
	"context"
	"math"
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/async"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Renders upcoming voices off the dispatch goroutine so dispatch never pays a cold render. The gate is
// VoiceCached under the CURRENT key, recomputed every call: never reintroduce a persistent seen-set, which kept edited voices cold.
type voicePrewarmer struct {
	pool     *async.Pool
	poolName string
	private  bool // pool is a private fallback (not registry-owned)

	// Tests inject a recorder instead of rendering.
	warmFn func(id instrument.ID, pitch float64)

	// Tests inject a recorder.
	warmParamsFn func(id instrument.ID, pitch float64, d instrument.Doc)

	// Melodic voices render per pitch, so neighbours need their own renders. ticks 0 disables the neighbourhood.
	isMelodic func(id instrument.ID) bool
	ticks     int

	mu sync.Mutex
	// So reWarmInstrument can replay them without touching Game state from the invalidation goroutine.
	pitchesByInst map[instrument.ID][]float64

	// Latest-wins: a dial turn publishes a document per detent, and one job each would fill the queue with unheard
	// sounds and drop the last.
	bindings map[warmKey]*bindingWarm

	renderDocFn func(id instrument.ID, pitch float64, d instrument.Doc)

	g *Game
}

// An unqueued slot had its submit refused and waits for retryBindings.
type bindingWarm struct {
	doc    instrument.Doc
	queued bool
}

// Within-pass dedup only, NOT a persistent warmed set.
type warmKey struct {
	id    instrument.ID
	pitch float64
}

// Must mirror audio.roundPitchForCache: the nearest 0.5 semitone.
func roundWarmPitch(p float64) float64 { return math.Round(p*2) / 2 }

const prewarmPoolName = "audio.prewarm"

func newVoicePrewarmer(g *Game) *voicePrewarmer {
	// Apply the memory ceiling once, before any warming can mint keys.
	if b := RuntimeProf().VoiceCacheBudgetBytes; b > 0 {
		audio.SetVoiceCacheBudgetBytes(b)
	}
	p := &voicePrewarmer{
		poolName:      prewarmPoolName,
		pitchesByInst: make(map[instrument.ID][]float64),
		bindings:      make(map[warmKey]*bindingWarm),
		renderDocFn:   audio.PrewarmVoiceDoc,
		isMelodic:     audio.WarmsPerPitch,
		ticks:         RuntimeProf().PredictiveWarmTicks,
		g:             g,
	}
	// The gated warm-up feeds this pool blocking, so the worker count is the gate's wall-clock divisor.
	workers := RuntimeProf().PrewarmWorkers
	if workers < 1 {
		workers = 1
	}
	opts := async.Options{MaxConcurrent: workers, QueueSize: 64, Name: prewarmPoolName}
	pool, err := async.DefaultRegistry().Get(prewarmPoolName, opts)
	if err != nil {
		pool = async.NewPool(nil, opts)
		p.private = true
	}
	p.pool = pool
	p.warmFn = func(id instrument.ID, pitch float64) {
		// Non-blocking: a dropped warm just means an inline render later, never a block.
		_ = p.pool.Submit(p.thenRetry(func() { audio.PrewarmVoice(id, pitch) }))
	}
	p.warmParamsFn = p.warmLatest
	// Arms invariant I3: from here the dispatch goroutine never calls recipe.Render.
	audio.SetAsyncRenderSubmit(func(id instrument.ID, pitch float64, d instrument.Doc) {
		_ = p.pool.Submit(p.thenRetry(func() { audio.PrewarmVoiceDoc(id, pitch, d) }))
	})
	audio.SetDispatchMustNotRender(true)
	// So the edited voice renders before the next beat instead of inline.
	audio.VoiceCacheInvalidateObserver = func(id instrument.ID) { p.reWarmInstrument(id) }
	// Warm the new binding whenever a param edit rotates an instrument's key.
	audio.InstrumentBindingChangedObserver = func(id instrument.ID, d instrument.Doc) {
		p.warmBinding(id, d)
	}
	return p
}

// Cheap every beat: a warm pitch costs a VoiceCached probe. Goroutine-safe.
func (p *voicePrewarmer) warmPitch(id instrument.ID, pitch float64) {
	if id == "" || p.warmFn == nil {
		return
	}
	if audio.VoiceCached(id, pitch) {
		return
	}
	p.warmFn(id, roundWarmPitch(pitch))
}

// Melodic only (others pitch after render); 0.5 st steps, the cache's resolution, clamped to ±24.
func (p *voicePrewarmer) pitchNeighborhood(pitch float64, melodic bool) []float64 {
	if !melodic || p.ticks <= 0 {
		return []float64{pitch}
	}
	out := make([]float64, 0, 2*p.ticks+1)
	for k := -p.ticks; k <= p.ticks; k++ {
		np := pitch + float64(k)*0.5
		if np < -24 || np > 24 {
			continue
		}
		out = append(out, np)
	}
	return out
}

// Pure param math, no render; a within-pass set collapses a looping melody's repeats so an edit cannot flood the
// pool. UI goroutine.
func (p *voicePrewarmer) warmFromBeatInfos() {
	g := p.g
	if g == nil || g.drum == nil {
		return
	}
	pass := make(map[warmKey]struct{})
	byInst := make(map[instrument.ID][]float64)
	rows := g.beatInfosByRow
	for row := range rows {
		if row >= len(g.drum.Rows) {
			break
		}
		inst := g.drum.Rows[row].Instrument
		if inst == "" {
			continue
		}
		melodic := p.isMelodic != nil && p.isMelodic(inst)
		for idx, info := range rows[row] {
			if !info.NodeType.Fires() {
				continue // mute/other node types don't schedule a voice
			}
			_, pitch, _ := g.evalNodeParamsOnly(row, idx, info)
			// The neighbourhood lets a one-tick pitch nudge land warm.
			for _, np := range p.pitchNeighborhood(pitch, melodic) {
				rp := roundWarmPitch(np)
				k := warmKey{id: inst, pitch: rp}
				if _, dup := pass[k]; dup {
					continue
				}
				pass[k] = struct{}{}
				byInst[inst] = append(byInst[inst], rp)
				p.warmPitch(inst, np)
			}
		}
	}
	p.mu.Lock()
	p.pitchesByInst = byInst
	p.mu.Unlock()
}

// Reads only the prewarmer's own state, so it is safe from the invalidation goroutine; id "" means all.
func (p *voicePrewarmer) reWarmInstrument(id instrument.ID) {
	p.mu.Lock()
	var work map[instrument.ID][]float64
	if id == "" {
		work = make(map[instrument.ID][]float64, len(p.pitchesByInst))
		for k, v := range p.pitchesByInst {
			work[k] = v
		}
	} else if v, ok := p.pitchesByInst[id]; ok {
		work = map[instrument.ID][]float64{id: v}
	}
	p.mu.Unlock()
	for inst, pitches := range work {
		for _, pitch := range pitches {
			p.warmPitch(inst, pitch)
		}
	}
}

// Reads only the prewarmer's own enumeration, so it is safe from the edit goroutine (possibly under seqMu).
func (p *voicePrewarmer) warmBinding(id instrument.ID, d instrument.Doc) {
	if id == "" || p.warmParamsFn == nil {
		return
	}
	p.mu.Lock()
	pitches := append([]float64(nil), p.pitchesByInst[id]...)
	p.mu.Unlock()
	for _, pitch := range pitches {
		p.warmParamsFn(id, pitch, d)
	}
}

// warmLatest never blocks: it runs on the UI goroutine, possibly under seqMu.
func (p *voicePrewarmer) warmLatest(id instrument.ID, pitch float64, d instrument.Doc) {
	k := warmKey{id: id, pitch: pitch}
	p.mu.Lock()
	b := p.bindings[k]
	if b == nil {
		b = &bindingWarm{}
		p.bindings[k] = b
	}
	b.doc = d
	if b.queued {
		p.mu.Unlock()
		return
	}
	b.queued = true
	p.mu.Unlock()
	p.submitBinding(k)
}

func (p *voicePrewarmer) submitBinding(k warmKey) {
	err := p.pool.Submit(p.thenRetry(func() { p.runBinding(k) }))
	if err == nil {
		return
	}
	p.mu.Lock()
	if b := p.bindings[k]; b != nil {
		if err == async.ErrClosed {
			delete(p.bindings, k)
		} else {
			b.queued = false
		}
	}
	p.mu.Unlock()
}

// The slot is removed before rendering so an edit landing mid-render queues a fresh job.
func (p *voicePrewarmer) runBinding(k warmKey) {
	p.mu.Lock()
	b := p.bindings[k]
	delete(p.bindings, k)
	p.mu.Unlock()
	if b == nil || p.renderDocFn == nil {
		return
	}
	p.renderDocFn(k.id, k.pitch, b.doc)
}

// At the end of each job, exactly when a queue slot opened.
func (p *voicePrewarmer) retryBindings() {
	p.mu.Lock()
	var retry []warmKey
	for k, b := range p.bindings {
		if !b.queued {
			b.queued = true
			retry = append(retry, k)
		}
	}
	p.mu.Unlock()
	for _, k := range retry {
		p.submitBinding(k)
	}
}

func (p *voicePrewarmer) thenRetry(job func()) async.Job {
	return func(context.Context) {
		job()
		p.retryBindings()
	}
}

// A private fallback pool is closed, not released: the registry does not own it.
func (p *voicePrewarmer) release() {
	if p == nil || p.pool == nil {
		return
	}
	audio.VoiceCacheInvalidateObserver = nil
	audio.InstrumentBindingChangedObserver = nil
	audio.SetDispatchMustNotRender(false)
	audio.SetAsyncRenderSubmit(nil)
	if p.private {
		p.pool.Close()
		return
	}
	async.DefaultRegistry().Release(p.poolName)
}
