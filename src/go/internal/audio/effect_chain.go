package audio

import (
	"math"
	"sync"
	"sync/atomic"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

var insertFXPushes struct{ structural, params atomic.Int64 }

// structural = whole-chain pushes (on WASM each rebuilds and rewires every effect on the channel), params = in-place
// updates of one live effect. A param gesture must move only params, and only on a value change.
func InsertFXPushCounts() (structural, params int64) {
	return insertFXPushes.structural.Load(), insertFXPushes.params.Load()
}

// uids[i] names slot i's processor INSTANCE: a structural edit keeps every surviving instance and its state on
// desktop and the worklet reuses its C instance by uid, so no edit resets a neighbour on one platform only.
type effectChainEntry struct {
	slots      []EffectSlot
	processors []InsertEffect
	uids       []uint64
}

// insertFXUIDs mints effect instance uids (never reused in a session).
var insertFXUIDs atomic.Uint64

// Read by the audio thread, written by the UI thread.
type effectChainManager struct {
	mu     sync.RWMutex
	chains map[channel.ID]*effectChainEntry
	sr     int
}

var insertChainMgr = &effectChainManager{
	chains: make(map[channel.ID]*effectChainEntry),
	// The engine rate until InitInsertChains sets the device's, so an offline render builds effects at the mixer's rate.
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
	// Real-time through SetParam; the platform gets the same in-place edit, since a whole-chain push would rebuild every
	// effect on WASM.
	insertFXPushes.params.Add(1)
	platformInsertEffectParamChanged(ch, e.uids[slotIndex], param, value)
}

// The worklet reconciles its C instances by uid.
func notifyInsertChainRebuilt(ch channel.ID, e *effectChainEntry) {
	insertFXPushes.structural.Add(1)
	if e == nil {
		platformInsertEffectsChanged(ch, nil)
		return
	}
	platformInsertEffectsChanged(ch, insertChainPush(e))
}

// COMPLETE params, defaults filled: the browser applies exactly these, never a default of its own.
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

// Read every frame by the FX chip, so it allocates nothing.
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

// The project-import boundary, so params are sanitised: a hand-edited file must not feed out-of-range or non-finite
// values into the C effects.
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

// Declared keys clamp to [Min,Max], non-finite ones fall back to Default; undeclared keys are kept when finite
// (newer builds) and dropped otherwise. Unknown types copy through: they render as passthrough.
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

// Called on playback stop.
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

// Also rebuilds every affected channel and notifies the platform: plain ClearAllInsertEffects leaves the built
// processors and the JS chain running stale.
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
		// No chain left: installs EQ-only processors and notifies with nil.
		rebuildChannelProcessors(id)
	}
}

// A no-op on desktop, where the Go processor list is the chain.
var platformInsertEffectsChanged = func(id channel.ID, chain []InsertEffectInstance) {}

// On WASM the worklet retunes that live C instance (state kept); on desktop the processor already took it via
// SetParam.
var platformInsertEffectParamChanged = func(id channel.ID, uid uint64, param string, value float64) {}

// Must hold insertChainMgr.mu. Inserts and EQ go separately to replaceProcessors so the mixer can tap between them.
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

	// Each channel keeps its own EQ record, so an insert-chain change never wipes another channel's EQ.
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

// Preserves the insert effects when the EQ changes.
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
