package ui

import (
	"context"
	"math"
	"sync"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/async"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// voicePrewarmer renders upcoming melody voices into audio's globalVoiceCache
// off the audioLoop dispatch goroutine, so dispatch never pays a cold synth
// render (the ~64ms church-organ render that made a beat land overdue and
// panicked the timing gate — see
// docs/superpowers/specs/2026-07-13-voice-prewarm-off-thread-design.md).
//
// Producers of warm requests:
//   - warmFromBeatInfos: graph-change enumeration of each row's node pitches.
//   - the scheduleSound backstop: the exact post-effect pitch (group/logic).
//   - reWarmInstrument: proactive re-warm when the cache is invalidated (a synth
//     param edit clears an instrument), replaying its last-enumerated pitches so
//     the pool starts rendering the new-params voice immediately instead of
//     leaving the next beat to cold-render inline.
//
// Correctness gate: warmPitch submits a warm iff audio.VoiceCached reports the
// voice is NOT already in the cache under the key dispatch will read. That key
// is recomputed from the CURRENT bpm+params on every call, so a param edit / BPM
// change / cache invalidation that moves the key is re-warmed automatically. An
// earlier version deduped on a static (id, pitch) "seen" set that omitted
// bpm+paramsHash: after a synth param edit cleared the cache, the pitch stayed
// marked warmed and was never re-rendered, so dispatch cold-rendered it and
// tripped the overdue gate. Never reintroduce a persistent seen-set as the
// correctness gate.
type voicePrewarmer struct {
	pool     *async.Pool
	poolName string
	private  bool // pool is a private fallback (not registry-owned)

	// warmFn performs one warm. Production submits audio.PrewarmVoice to the
	// pool; tests inject a recorder to observe decisions without rendering.
	warmFn func(id string, pitch float64)

	// warmParamsFn warms an EXPLICIT binding. Production submits
	// audio.PrewarmVoiceParams to the pool; tests inject a recorder.
	warmParamsFn func(id string, pitch float64, params audio.RecipeParams)

	// isMelodic reports whether an instrument renders at-pitch (so pitch
	// neighbors need separate renders). Injectable for tests. ticks is the ±N
	// pitch-neighborhood radius (0 disables).
	isMelodic func(id string) bool
	ticks     int

	mu sync.Mutex
	// pitchesByInst records the distinct rounded pitches last enumerated per
	// instrument, so reWarmInstrument can replay them on a cache invalidation
	// without touching Game state from the invalidation goroutine.
	pitchesByInst map[string][]float64

	g *Game
}

// warmKey identifies a distinct (instrument, rounded-pitch) pair. Used for
// within-pass dedup in warmFromBeatInfos — NOT as a persistent warmed-set.
type warmKey struct {
	id    string
	pitch float64
}

// roundWarmPitch mirrors audio.roundPitchForCache (unexported there): round to
// the nearest 0.5 semitone.
func roundWarmPitch(p float64) float64 { return math.Round(p*2) / 2 }

const prewarmPoolName = "audio.prewarm"

func newVoicePrewarmer(g *Game) *voicePrewarmer {
	// Apply the memory ceiling once, before any warming can mint keys.
	if b := RuntimeProf().VoiceCacheBudgetBytes; b > 0 {
		audio.SetVoiceCacheBudgetBytes(b)
	}
	p := &voicePrewarmer{
		poolName:      prewarmPoolName,
		pitchesByInst: make(map[string][]float64),
		isMelodic:     audio.IsMelodicInstrument,
		ticks:         RuntimeProf().PredictiveWarmTicks,
		g:             g,
	}
	// 2 workers so a burst of distinct pitches warms in parallel without
	// starving the single audio.timers worker; queue 64 mirrors audioCh depth.
	opts := async.Options{MaxConcurrent: 2, QueueSize: 64, Name: prewarmPoolName}
	pool, err := async.DefaultRegistry().Get(prewarmPoolName, opts)
	if err != nil {
		pool = async.NewPool(nil, opts)
		p.private = true
	}
	p.pool = pool
	p.warmFn = func(id string, pitch float64) {
		// Non-blocking: on backpressure the warm is dropped and dispatch
		// degrades to today's inline render — never worse, never blocks.
		_ = p.pool.Submit(func(context.Context) { audio.PrewarmVoice(id, pitch) })
	}
	p.warmParamsFn = func(id string, pitch float64, params audio.RecipeParams) {
		// Non-blocking: on backpressure the warm is dropped and dispatch
		// degrades to the best-available fallback — never worse, never blocks.
		_ = p.pool.Submit(func(context.Context) { audio.PrewarmVoiceParams(id, pitch, params) })
	}
	// Hand exact renders to the prewarm pool and arm invariant I3: from here
	// on, the audioLoop dispatch goroutine must never call recipe.Render.
	audio.SetAsyncRenderSubmit(func(id string, pitch float64, params audio.RecipeParams) {
		_ = p.pool.Submit(func(context.Context) { audio.PrewarmVoiceParams(id, pitch, params) })
	})
	audio.SetDispatchMustNotRender(true)
	// Proactively re-warm on cache invalidation (synth param edit / reset), so
	// the new-params voice renders before the next beat instead of inline.
	audio.VoiceCacheInvalidateObserver = func(id string) { p.reWarmInstrument(id) }
	// Warm the new binding whenever a param edit rotates an instrument's key.
	audio.InstrumentBindingChangedObserver = func(id string, params audio.RecipeParams) {
		p.warmBinding(id, params)
	}
	return p
}

// warmPitch submits one warm for (id, pitch) iff it is not already cached under
// the key dispatch will read. Safe to call every beat / every enumeration:
// already-warm pitches are a cheap VoiceCached probe and no submit. Goroutine-
// safe (VoiceCached is mutex-guarded in audio; warmFn submits to a safe pool).
func (p *voicePrewarmer) warmPitch(id string, pitch float64) {
	if id == "" || p.warmFn == nil {
		return
	}
	if audio.VoiceCached(id, pitch) {
		return
	}
	p.warmFn(id, roundWarmPitch(pitch))
}

// pitchNeighborhood returns the pitch plus, for melodic instruments, ±ticks
// steps of 0.5st (the cache's pitch resolution) around it. Non-melodic
// instruments (pitch is post-render) get just the exact pitch. Values are
// clamped to the legal [-24, 24] semitone range.
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

// warmFromBeatInfos enumerates each row's firing pitches from the current
// beatInfosByRow window and warms the distinct (rowInstrument, pitch) pairs.
// evalNodeParamsOnly is pure param math (node pitch + windowed group transpose,
// NO render), so this is cheap; the render is what warmPitch offloads. Runs on
// the UI goroutine (updateBeatInfos caller). A local within-pass set collapses
// the many repeated beats of a looping melody to one warmPitch per distinct
// pair, so a graph edit can't burst-flood the pool. Records the enumerated
// pitches per instrument for reWarmInstrument.
func (p *voicePrewarmer) warmFromBeatInfos() {
	g := p.g
	if g == nil || g.drum == nil {
		return
	}
	pass := make(map[warmKey]struct{})
	byInst := make(map[string][]float64)
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
			if info.NodeType != model.NodeTypeRegular {
				continue // mute/other node types don't schedule a voice
			}
			_, pitch, _ := g.evalNodeParamsOnly(row, idx, info)
			// The exact node pitch, plus (melodic instruments only) the
			// ±ticks × 0.5st neighborhood so an immediate one-tick pitch nudge
			// lands on a warm cache instead of a ~24ms cold render. Non-melodic
			// pitch is a post-render resample (already a hit), so no neighbors.
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

// reWarmInstrument replays the last-enumerated pitches for id (all instruments
// when id==""), re-warming any that the invalidation left cold. Reads only the
// prewarmer's own recorded state, so it is safe to call from the audio cache-
// invalidation goroutine. warmPitch's VoiceCached gate makes already-warm
// pitches cheap no-ops.
func (p *voicePrewarmer) reWarmInstrument(id string) {
	p.mu.Lock()
	var work map[string][]float64
	if id == "" {
		work = make(map[string][]float64, len(p.pitchesByInst))
		for k, v := range p.pitchesByInst {
			work[k] = v
		}
	} else if v, ok := p.pitchesByInst[id]; ok {
		work = map[string][]float64{id: v}
	}
	p.mu.Unlock()
	for inst, pitches := range work {
		for _, pitch := range pitches {
			p.warmPitch(inst, pitch)
		}
	}
}

// warmBinding renders the binding implied by params at every pitch id is
// currently enumerated to play, off-thread, so the next trigger is a cache HIT
// rather than a ~24ms cold render on the audio dispatch goroutine.
//
// Reads only the prewarmer's own recorded pitch enumeration, so it is safe to
// call from the edit goroutine (which is the UI goroutine, possibly under
// seqMu) without touching Game state.
func (p *voicePrewarmer) warmBinding(id string, params audio.RecipeParams) {
	if id == "" || p.warmParamsFn == nil {
		return
	}
	p.mu.Lock()
	pitches := append([]float64(nil), p.pitchesByInst[id]...)
	p.mu.Unlock()
	for _, pitch := range pitches {
		p.warmParamsFn(id, pitch, params)
	}
}

// release frees the registry budget for the prewarm pool (closes a private
// fallback pool, which is not registry-owned) and unhooks the invalidation
// observer.
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
