package audio

import "sync"

// effectChainEntry holds the insert effect chain for one instrument.
type effectChainEntry struct {
	slots      []EffectSlot
	processors []InsertEffect
}

// effectChainManager stores per-instrument insert effect chains.
// Thread-safe for concurrent reads (audio thread) and writes (UI thread).
type effectChainManager struct {
	mu     sync.RWMutex
	chains map[string]*effectChainEntry
	sr     int
}

var insertChainMgr = &effectChainManager{
	chains: make(map[string]*effectChainEntry),
	sr:     44100,
}

// InitInsertChains sets the sample rate for new effect processors.
func InitInsertChains(sr int) {
	insertChainMgr.mu.Lock()
	insertChainMgr.sr = sr
	insertChainMgr.mu.Unlock()
}

func (m *effectChainManager) ensureEntry(id string) *effectChainEntry {
	if e := m.chains[id]; e != nil {
		return e
	}
	e := &effectChainEntry{}
	m.chains[id] = e
	return e
}

// AddInsertEffect adds a new effect to an instrument's chain. Returns the slot index.
func AddInsertEffect(instrumentID string, effectType EffectType, params map[string]float64) int {
	m := insertChainMgr
	m.mu.Lock()
	defer m.mu.Unlock()

	e := m.ensureEntry(instrumentID)
	if params == nil {
		params = DefaultParams(effectType)
	}
	slot := EffectSlot{Type: effectType, Enabled: true, Params: params}
	e.slots = append(e.slots, slot)

	proc := NewEffectProcessor(slot, m.sr)
	e.processors = append(e.processors, proc)

	rebuildChannelProcessors(instrumentID)
	return len(e.slots) - 1
}

// RemoveInsertEffect removes an effect at the given slot index.
func RemoveInsertEffect(instrumentID string, slotIndex int) {
	m := insertChainMgr
	m.mu.Lock()
	defer m.mu.Unlock()

	e := m.chains[instrumentID]
	if e == nil || slotIndex < 0 || slotIndex >= len(e.slots) {
		return
	}
	e.slots = append(e.slots[:slotIndex], e.slots[slotIndex+1:]...)
	e.processors = append(e.processors[:slotIndex], e.processors[slotIndex+1:]...)

	rebuildChannelProcessors(instrumentID)
}

// MoveInsertEffect moves an effect from one position to another.
func MoveInsertEffect(instrumentID string, fromIndex, toIndex int) {
	m := insertChainMgr
	m.mu.Lock()
	defer m.mu.Unlock()

	e := m.chains[instrumentID]
	if e == nil || fromIndex < 0 || fromIndex >= len(e.slots) ||
		toIndex < 0 || toIndex >= len(e.slots) || fromIndex == toIndex {
		return
	}

	slot := e.slots[fromIndex]
	proc := e.processors[fromIndex]

	// Remove from old position
	e.slots = append(e.slots[:fromIndex], e.slots[fromIndex+1:]...)
	e.processors = append(e.processors[:fromIndex], e.processors[fromIndex+1:]...)

	// Insert at new position
	e.slots = append(e.slots[:toIndex], append([]EffectSlot{slot}, e.slots[toIndex:]...)...)
	e.processors = append(e.processors[:toIndex], append([]InsertEffect{proc}, e.processors[toIndex:]...)...)

	rebuildChannelProcessors(instrumentID)
}

// SetInsertEffectParam updates a parameter on a specific effect slot.
func SetInsertEffectParam(instrumentID string, slotIndex int, param string, value float64) {
	m := insertChainMgr
	m.mu.Lock()
	defer m.mu.Unlock()

	e := m.chains[instrumentID]
	if e == nil || slotIndex < 0 || slotIndex >= len(e.slots) {
		return
	}
	e.slots[slotIndex].Params[param] = value
	e.processors[slotIndex].SetParam(param, value)
	// No need to rebuild channel processors — param changes are real-time via SetParam.
	// But WASM needs to know about the param change.
	platformInsertEffectsChanged(instrumentID, e.slots)
}

// ToggleInsertEffect enables or disables an effect slot.
func ToggleInsertEffect(instrumentID string, slotIndex int, enabled bool) {
	m := insertChainMgr
	m.mu.Lock()
	defer m.mu.Unlock()

	e := m.chains[instrumentID]
	if e == nil || slotIndex < 0 || slotIndex >= len(e.slots) {
		return
	}
	e.slots[slotIndex].Enabled = enabled

	rebuildChannelProcessors(instrumentID)
}

// GetInsertEffects returns a copy of the current effect slots for an instrument.
func GetInsertEffects(instrumentID string) []EffectSlot {
	m := insertChainMgr
	m.mu.RLock()
	defer m.mu.RUnlock()

	e := m.chains[instrumentID]
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

// SetInsertEffects replaces the entire insert chain for an instrument.
// Used during import to restore saved state. This is the project-import
// boundary, so each slot's params are sanitized against the effect registry
// (sanitizeEffectParams) — a hand-edited or corrupted beatmo.json must not
// feed out-of-range or non-finite values into the C effect processors.
func SetInsertEffects(instrumentID string, slots []EffectSlot) {
	m := insertChainMgr
	m.mu.Lock()
	defer m.mu.Unlock()

	e := m.ensureEntry(instrumentID)
	e.slots = make([]EffectSlot, len(slots))
	e.processors = make([]InsertEffect, len(slots))
	for i, s := range slots {
		e.slots[i] = EffectSlot{
			Type:    s.Type,
			Enabled: s.Enabled,
			Params:  sanitizeEffectParams(s.Type, s.Params),
		}
		e.processors[i] = NewEffectProcessor(e.slots[i], m.sr)
	}

	rebuildChannelProcessors(instrumentID)
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
	m.chains = make(map[string]*effectChainEntry)
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
	ids := make([]string, 0, len(m.chains))
	for id := range m.chains {
		ids = append(ids, id)
	}
	m.chains = make(map[string]*effectChainEntry)
	for _, id := range ids {
		// chains[id] is now gone → rebuild installs EQ-only processors and
		// fires platformInsertEffectsChanged(id, nil).
		rebuildChannelProcessors(id)
	}
}

// platformInsertEffectsChanged is called after any insert chain mutation
// to propagate the change to the platform audio system. On WASM this pushes
// the slot list to WebAudio; on desktop it's a no-op (Go processors handle it).
var platformInsertEffectsChanged = func(id string, slots []EffectSlot) {}

// rebuildChannelProcessors merges insert effects + EQ into the channel's
// processor chain. Must be called with insertChainMgr.mu held.
//
// The chain order is: [enabled inserts...] → [EQ processor]. The insert and
// EQ lists are passed separately to Channel.replaceProcessors so the mixer
// can tap the boundary as scope.StageInsertFX.
func rebuildChannelProcessors(instrumentID string) {
	var inserts []Processor
	var eqProcs []Processor

	// Collect enabled insert effects.
	e := insertChainMgr.chains[instrumentID]
	if e != nil {
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
	if eq := lookupChannelEQ(instrumentID); len(eq.Bands) > 0 {
		eqProcs = append(eqProcs, NewEQProcessor(eq.SampleRate, eq.Bands...))
	}

	ch := chanMgr.ensureChannel(instrumentID)
	ch.replaceProcessors(inserts, eqProcs)

	// Notify platform (WASM → JS WebAudio)
	e2 := insertChainMgr.chains[instrumentID]
	if e2 != nil {
		platformInsertEffectsChanged(instrumentID, e2.slots)
	} else {
		platformInsertEffectsChanged(instrumentID, nil)
	}
}

// RebuildChannelWithEQ is called when EQ changes to ensure insert effects
// are preserved in the processor chain. This replaces SetChannelProcessors
// for channels that may have insert effects.
func RebuildChannelWithEQ(id string, eqProc Processor) {
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
