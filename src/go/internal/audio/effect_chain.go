package audio

import (
	"math"
	"sync"
	"sync/atomic"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// insertFXPushes counts what insert-chain edits sent to the platform audio
// graph (see InsertFXPushCounts).
var insertFXPushes struct{ structural, params atomic.Int64 }

// InsertFXPushCounts reports how many insert-chain edits reached the platform
// audio graph since process start: structural = whole-chain rebuilds (on WASM
// every one builds fresh WebAudio nodes for EVERY effect on the channel and
// rewires it, resetting delay lines and LFO phases), params = in-place
// updates of one live effect. A param gesture must move only params, and
// only when a value changed. Perf guards and benches read it.
func InsertFXPushCounts() (structural, params int64) {
	return insertFXPushes.structural.Load(), insertFXPushes.params.Load()
}

// effectChainEntry holds the insert effect chain for one instrument. uids[i]
// names slot i's processor INSTANCE: a structural edit (add, remove, move,
// toggle) keeps every surviving instance and its state on desktop, and the
// browser's worklet reuses its C instance by the same uid, so an edit never
// resets a neighbour's delay line or LFO on one platform only.
type effectChainEntry struct {
	slots      []EffectSlot
	processors []InsertEffect
	uids       []uint64
}

// insertFXUIDs mints effect instance uids (never reused in a session).
var insertFXUIDs atomic.Uint64

// effectChainManager stores per-instrument insert effect chains.
// Thread-safe for concurrent reads (audio thread) and writes (UI thread).
type effectChainManager struct {
	mu     sync.RWMutex
	chains map[channel.ID]*effectChainEntry
	sr     int
}

var insertChainMgr = &effectChainManager{
	chains: make(map[channel.ID]*effectChainEntry),
	// The engine rate before the device opens (InitInsertChains then sets the
	// rate it really runs at): an offline render with no device used to build
	// every effect for 44.1 kHz inside a 48 kHz mixer.
	sr: EngineSampleRate,
}

// InitInsertChains sets the sample rate for new effect processors.
func InitInsertChains(sr int) {
	insertChainMgr.mu.Lock()
	insertChainMgr.sr = sr
	insertChainMgr.mu.Unlock()
}

func (m *effectChainManager) ensureEntry(id channel.ID) *effectChainEntry {
	if e := m.chains[id]; e != nil {
		return e
	}
	e := &effectChainEntry{}
	m.chains[id] = e
	return e
}

// AddInsertEffect adds a new effect to an instrument's chain. Returns the slot index.
func AddInsertEffect(inst instrument.ID, effectType EffectType, params map[string]float64) int {
	ch := channel.Of(inst)
	m := insertChainMgr
	m.mu.Lock()
	defer m.mu.Unlock()

	e := m.ensureEntry(ch)
	if params == nil {
		params = DefaultParams(effectType)
	}
	slot := EffectSlot{Type: effectType, Enabled: true, Params: params}
	e.slots = append(e.slots, slot)

	proc := NewEffectProcessor(slot, m.sr)
	e.processors = append(e.processors, proc)
	e.uids = append(e.uids, insertFXUIDs.Add(1))

	rebuildChannelProcessors(ch)
	return len(e.slots) - 1
}

// RemoveInsertEffect removes an effect at the given slot index.
func RemoveInsertEffect(inst instrument.ID, slotIndex int) {
	ch := channel.Of(inst)
	m := insertChainMgr
	m.mu.Lock()
	defer m.mu.Unlock()

	e := m.chains[ch]
	if e == nil || slotIndex < 0 || slotIndex >= len(e.slots) {
		return
	}
	e.slots = append(e.slots[:slotIndex], e.slots[slotIndex+1:]...)
	e.processors = append(e.processors[:slotIndex], e.processors[slotIndex+1:]...)
	e.uids = append(e.uids[:slotIndex], e.uids[slotIndex+1:]...)

	rebuildChannelProcessors(ch)
}

// MoveInsertEffect moves an effect from one position to another.
func MoveInsertEffect(inst instrument.ID, fromIndex, toIndex int) {
	ch := channel.Of(inst)
	m := insertChainMgr
	m.mu.Lock()
	defer m.mu.Unlock()

	e := m.chains[ch]
	if e == nil || fromIndex < 0 || fromIndex >= len(e.slots) ||
		toIndex < 0 || toIndex >= len(e.slots) || fromIndex == toIndex {
		return
	}

	slot := e.slots[fromIndex]
	proc := e.processors[fromIndex]
	uid := e.uids[fromIndex]

	// Remove from old position
	e.slots = append(e.slots[:fromIndex], e.slots[fromIndex+1:]...)
	e.processors = append(e.processors[:fromIndex], e.processors[fromIndex+1:]...)
	e.uids = append(e.uids[:fromIndex], e.uids[fromIndex+1:]...)

	// Insert at new position
	e.slots = append(e.slots[:toIndex], append([]EffectSlot{slot}, e.slots[toIndex:]...)...)
	e.processors = append(e.processors[:toIndex], append([]InsertEffect{proc}, e.processors[toIndex:]...)...)
	e.uids = append(e.uids[:toIndex], append([]uint64{uid}, e.uids[toIndex:]...)...)

	rebuildChannelProcessors(ch)
}

// SetInsertEffectParam updates a parameter on a specific effect slot.
func SetInsertEffectParam(inst instrument.ID, slotIndex int, param string, value float64) {
	ch := channel.Of(inst)
	m := insertChainMgr
	m.mu.Lock()
	defer m.mu.Unlock()

	e := m.chains[ch]
	if e == nil || slotIndex < 0 || slotIndex >= len(e.slots) {
		return
	}
	e.slots[slotIndex].Params[param] = value
	e.processors[slotIndex].SetParam(param, value)
	// No need to rebuild channel processors — param changes are real-time via
	// SetParam. The platform graph gets the same in-place edit: on WASM a
	// whole-chain push would rebuild every effect on the channel.
	insertFXPushes.params.Add(1)
	platformInsertEffectParamChanged(ch, e.uids[slotIndex], param, value)
}

// notifyInsertChainRebuilt pushes a channel's whole slot list to the platform
// (WASM: the worklet reconciles its C instances by uid) and counts it.
func notifyInsertChainRebuilt(ch channel.ID, e *effectChainEntry) {
	insertFXPushes.structural.Add(1)
	if e == nil {
		platformInsertEffectsChanged(ch, nil)
		return
	}
	platformInsertEffectsChanged(ch, insertChainPush(e))
}

// InsertEffectInstance is one slot as the platform builds it: the instance
// uid, the type, whether it runs, and its COMPLETE parameter set (defaults
// filled — the browser applies exactly these values, never a default of its
// own).
type InsertEffectInstance struct {
	UID     uint64             `json:"uid"`
	Type    EffectType         `json:"type"`
	Enabled bool               `json:"enabled"`
	Params  map[string]float64 `json:"params"`
}

func insertChainPush(e *effectChainEntry) []InsertEffectInstance {
	out := make([]InsertEffectInstance, len(e.slots))
	for i, s := range e.slots {
		out[i] = InsertEffectInstance{UID: e.uids[i], Type: s.Type, Enabled: s.Enabled, Params: mergeDefaults(s.Type, s.Params)}
	}
	return out
}

// InsertChainForPlatform is the channel's insert chain exactly as it is pushed
// to the browser (cmd/export_audio's cross-platform cases use it).
func InsertChainForPlatform(inst instrument.ID) []InsertEffectInstance {
	m := insertChainMgr
	m.mu.RLock()
	defer m.mu.RUnlock()
	e := m.chains[channel.Of(inst)]
	if e == nil {
		return nil
	}
	return insertChainPush(e)
}

// ToggleInsertEffect enables or disables an effect slot.
func ToggleInsertEffect(inst instrument.ID, slotIndex int, enabled bool) {
	ch := channel.Of(inst)
	m := insertChainMgr
	m.mu.Lock()
	defer m.mu.Unlock()

	e := m.chains[ch]
	if e == nil || slotIndex < 0 || slotIndex >= len(e.slots) {
		return
	}
	e.slots[slotIndex].Enabled = enabled

	rebuildChannelProcessors(ch)
}

// GetInsertEffects returns a copy of the current effect slots for an instrument.
func GetInsertEffects(inst instrument.ID) []EffectSlot {
	ch := channel.Of(inst)
	m := insertChainMgr
	m.mu.RLock()
	defer m.mu.RUnlock()

	e := m.chains[ch]
	if e == nil {
		return []EffectSlot{}
	}
	out := make([]EffectSlot, len(e.slots))
	for i, s := range e.slots {
		out[i] = EffectSlot{
			Type:    s.Type,
			Enabled: s.Enabled,
			Params:  copyParams(s.Params),
		}
	}
	return out
}

// ActiveInsertEffectCount is how many of an instrument's insert effects are
// on (the row rack's FX chip; read every frame, so it allocates nothing).
func ActiveInsertEffectCount(inst instrument.ID) int {
	m := insertChainMgr
	m.mu.RLock()
	defer m.mu.RUnlock()
	n := 0
	if e := m.chains[channel.Of(inst)]; e != nil {
		for _, s := range e.slots {
			if s.Enabled {
				n++
			}
		}
	}
	return n
}

// SetInsertEffects replaces the entire insert chain for an instrument.
// Used during import to restore saved state. This is the project-import
// boundary, so each slot's params are sanitized against the effect registry
// (sanitizeEffectParams) — a hand-edited or corrupted beatmo.json must not
// feed out-of-range or non-finite values into the C effect processors.
func SetInsertEffects(inst instrument.ID, slots []EffectSlot) {
	ch := channel.Of(inst)
	m := insertChainMgr
	m.mu.Lock()
	defer m.mu.Unlock()

	e := m.ensureEntry(ch)
	e.slots = make([]EffectSlot, len(slots))
	e.processors = make([]InsertEffect, len(slots))
	e.uids = make([]uint64, len(slots))
	for i, s := range slots {
		e.slots[i] = EffectSlot{
			Type:    s.Type,
			Enabled: s.Enabled,
			Params:  sanitizeEffectParams(s.Type, s.Params),
		}
		e.processors[i] = NewEffectProcessor(e.slots[i], m.sr)
		e.uids[i] = insertFXUIDs.Add(1)
	}

	rebuildChannelProcessors(ch)
}

// sanitizeEffectParams returns a fresh copy of params with every key that
// the effect's registry entry declares clamped to its [Min,Max]; non-finite
// values fall back to the declared Default. Keys the registry doesn't
// declare are kept as-is when finite (forward-compat with newer builds) and
// dropped when non-finite. Unknown effect types (no registration) copy
// through untouched — they already render as passthrough processors.
func sanitizeEffectParams(t EffectType, params map[string]float64) map[string]float64 {
	if params == nil {
		return nil
	}
	registryMu.RLock()
	reg := registryMap[t]
	registryMu.RUnlock()
	out := make(map[string]float64, len(params))
	for k, v := range params {
		var def *EffectParamDef
		if reg != nil {
			for i := range reg.Params {
				if reg.Params[i].Name == k {
					def = &reg.Params[i]
					break
				}
			}
		}
		switch {
		case def == nil:
			if isFiniteParam(v) {
				out[k] = v
			}
		case !isFiniteParam(v):
			out[k] = def.Default
		case v < def.Min:
			out[k] = def.Min
		case v > def.Max:
			out[k] = def.Max
		default:
			out[k] = v
		}
	}
	return out
}

// ResetAllInsertEffects clears state (delay buffers, filter state) on all
// insert effect processors. Called on playback stop.
func ResetAllInsertEffects() {
	m := insertChainMgr
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, e := range m.chains {
		for _, p := range e.processors {
			p.Reset()
		}
	}
}

// ClearAllInsertEffects removes all insert chains (used on full reset).
func ClearAllInsertEffects() {
	m := insertChainMgr
	m.mu.Lock()
	defer m.mu.Unlock()
	m.chains = make(map[channel.ID]*effectChainEntry)
}

// ClearAllInsertEffectsAndNotify removes all insert chains AND rebuilds the
// processor chain for every formerly-affected instrument, notifying the
// platform layer (WASM → JS WebAudio) per id. Used by project import: a
// project that carries no effects for an instrument must silence whatever
// chain the previous project installed — both in the Go mixer and in the
// browser's worklet graph. Plain ClearAllInsertEffects drops the manager
// state but leaves the already-built channel processors (and the JS chain)
// running stale.
func ClearAllInsertEffectsAndNotify() {
	m := insertChainMgr
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]channel.ID, 0, len(m.chains))
	for id := range m.chains {
		ids = append(ids, id)
	}
	m.chains = make(map[channel.ID]*effectChainEntry)
	for _, id := range ids {
		// chains[id] is now gone → rebuild installs EQ-only processors and
		// fires platformInsertEffectsChanged(id, nil).
		rebuildChannelProcessors(id)
	}
}

// platformInsertEffectsChanged is called after any insert chain mutation
// to propagate the change to the platform audio system. On WASM this pushes
// the chain to the beatmo-dsp worklet; on desktop it's a no-op (the Go
// processor list already is the chain).
var platformInsertEffectsChanged = func(id channel.ID, chain []InsertEffectInstance) {}

// platformInsertEffectParamChanged is called after a param edit on ONE slot.
// On WASM the worklet retunes that live C instance (ifx_set, state kept); on
// desktop it's a no-op — the slot's processor already took the value through
// SetParam.
var platformInsertEffectParamChanged = func(id channel.ID, uid uint64, param string, value float64) {}

// rebuildChannelProcessors merges insert effects + EQ into the channel's
// processor chain. Must be called with insertChainMgr.mu held.
//
// The chain order is: [enabled inserts...] → [EQ processor]. The insert and
// EQ lists are passed separately to Channel.replaceProcessors so the mixer
// can tap the boundary as scope.StageInsertFX.
func rebuildChannelProcessors(ch channel.ID) {
	var inserts []Processor
	var eqProcs []Processor

	// Collect enabled insert effects.
	e := insertChainMgr.chains[ch]
	var slots []EffectSlot
	if e != nil {
		slots = e.slots
		for i, p := range e.processors {
			if e.slots[i].Enabled {
				inserts = append(inserts, p)
			}
		}
	}

	// Preserve the existing EQ for THIS channel. Pre-Phase-5 this used the
	// process-wide lastSetEQ and gated on ID matching, which silently lost
	// EQ on any instrument the user wasn't currently editing. Now each
	// channel keeps its own EQ record so insert-chain changes never wipe
	// another channel's EQ.
	if eq := lookupChannelEQ(ch); len(eq.Bands) > 0 {
		proc := eq.proc
		if proc == nil {
			proc = NewEQProcessor(eq.SampleRate, eq.Bands...)
		}
		eqProcs = append(eqProcs, proc)
	}

	c := chanMgr.ensureChannel(ch)
	c.memory.Store(math.Float64bits(ChainMemorySec(slots)))
	c.replaceProcessors(inserts, eqProcs)

	// Notify platform (WASM → the beatmo-dsp worklet)
	notifyInsertChainRebuilt(ch, insertChainMgr.chains[ch])
}

// RebuildChannelWithEQ is called when EQ changes to ensure insert effects
// are preserved in the processor chain. This replaces SetChannelProcessors
// for channels that may have insert effects.
func RebuildChannelWithEQ(id channel.ID, eqProc Processor) {
	m := insertChainMgr
	m.mu.RLock()
	defer m.mu.RUnlock()

	var inserts []Processor

	// Collect enabled insert effects.
	e := m.chains[id]
	if e != nil {
		for i, p := range e.processors {
			if e.slots[i].Enabled {
				inserts = append(inserts, p)
			}
		}
	}

	var eqProcs []Processor
	if eqProc != nil {
		eqProcs = append(eqProcs, eqProc)
	}

	ch := chanMgr.ensureChannel(id)
	ch.replaceProcessors(inserts, eqProcs)
}

func copyParams(m map[string]float64) map[string]float64 {
	if m == nil {
		return nil
	}
	out := make(map[string]float64, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
