package audio

import "sync"

// EffectRegistration describes an effect type for the registry.
type EffectRegistration struct {
	Type        EffectType
	DisplayName string
	Category    string // "modulation", "dynamics", "creative", "utility"
	Params      []EffectParamDef
	New         func(sr int, params map[string]float64) InsertEffect
}

var (
	registryMu    sync.RWMutex
	registryMap   = map[EffectType]*EffectRegistration{}
	registryOrder []EffectType
)

// RegisterEffect adds an effect type to the global registry.
// Call from init() in each effect file.
func RegisterEffect(reg EffectRegistration) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, exists := registryMap[reg.Type]; exists {
		// Update existing registration (e.g., platform-specific New override).
		registryMap[reg.Type].New = reg.New
		return
	}
	r := reg // copy
	registryMap[reg.Type] = &r
	registryOrder = append(registryOrder, reg.Type)
}

// EffectRegistrations returns all registered effect types.
func EffectRegistrations() map[EffectType]*EffectRegistration {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make(map[EffectType]*EffectRegistration, len(registryMap))
	for k, v := range registryMap {
		out[k] = v
	}
	return out
}

// EffectTypeOrder returns effect types in registration order.
func EffectTypeOrder() []EffectType {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]EffectType, len(registryOrder))
	copy(out, registryOrder)
	return out
}

// applyDefaultFX processes a float32 buffer through a chain of insert effects.
// Creates temporary processors, processes the buffer, then discards them.
// Used by CVariantInstrument.DefaultFX during voice rendering (cached).
func applyDefaultFX(buf []float32, sampleRate int, slots []EffectSlot) {
	if len(slots) == 0 {
		return
	}
	procs := make([]InsertEffect, 0, len(slots))
	for _, s := range slots {
		if s.Enabled {
			procs = append(procs, NewEffectProcessor(s, sampleRate))
		}
	}
	if len(procs) == 0 {
		return
	}
	for i, x := range buf {
		sample := float64(x)
		for _, p := range procs {
			sample = p.ProcessSample(sample)
		}
		buf[i] = float32(sample)
	}
}

// clampf constrains v to [lo, hi]. Shared by all effect implementations.
func clampf(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
