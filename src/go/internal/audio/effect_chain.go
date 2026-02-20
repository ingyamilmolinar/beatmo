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
// Used during import to restore saved state.
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
			Params:  copyParams(s.Params),
		}
		e.processors[i] = NewEffectProcessor(e.slots[i], m.sr)
	}

	rebuildChannelProcessors(instrumentID)
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

// platformInsertEffectsChanged is called after any insert chain mutation
// to propagate the change to the platform audio system. On WASM this pushes
// the slot list to WebAudio; on desktop it's a no-op (Go processors handle it).
var platformInsertEffectsChanged = func(id string, slots []EffectSlot) {}

// rebuildChannelProcessors merges insert effects + EQ into the channel's
// processor chain. Must be called with insertChainMgr.mu held.
//
// The chain order is: [enabled inserts...] → [EQ processor]
// This is pushed to the channel via replaceProcessors.
func rebuildChannelProcessors(instrumentID string) {
	var procs []Processor

	// Add enabled insert effects
	e := insertChainMgr.chains[instrumentID]
	if e != nil {
		for i, p := range e.processors {
			if e.slots[i].Enabled {
				procs = append(procs, p)
			}
		}
	}

	// Preserve the existing EQ: rebuild from lastSetEQ if it matches this channel.
	eq := lastSetEQ
	if eq.ID == instrumentID && len(eq.Bands) > 0 {
		procs = append(procs, NewEQProcessor(eq.SampleRate, eq.Bands...))
	}

	ch := chanMgr.ensureChannel(instrumentID)
	ch.replaceProcessors(procs)

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

	var procs []Processor

	// Add enabled insert effects
	e := m.chains[id]
	if e != nil {
		for i, p := range e.processors {
			if e.slots[i].Enabled {
				procs = append(procs, p)
			}
		}
	}

	// Add EQ
	if eqProc != nil {
		procs = append(procs, eqProc)
	}

	ch := chanMgr.ensureChannel(id)
	ch.replaceProcessors(procs)
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
