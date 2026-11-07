package audio

import "fmt"

// EffectType identifies a kind of insert effect.
type EffectType string

const (
	EffectDistortion EffectType = "distortion"
	EffectDelay      EffectType = "delay"
	EffectReverb     EffectType = "reverb"
	EffectChorus     EffectType = "chorus"
	EffectBitcrusher EffectType = "bitcrusher"
	EffectFilter     EffectType = "filter"

	// Modulation
	EffectPhaser  EffectType = "phaser"
	EffectFlanger EffectType = "flanger"
	EffectTremolo EffectType = "tremolo"

	// Dynamics
	EffectGate    EffectType = "gate"
	EffectLimiter EffectType = "limiter"

	// Creative
	EffectRingMod    EffectType = "ringmod"
	EffectWaveshaper EffectType = "waveshaper"
	EffectAutoWah    EffectType = "autowah"

	// New dynamics
	EffectCompressor EffectType = "compressor"
	EffectTransient  EffectType = "transient"

	// New creative
	EffectTape       EffectType = "tape"
	EffectPitchShift EffectType = "pitchshift"
)

// The closed set a file, a bridge call or a document may name.
var EffectTypes = []EffectType{
	EffectDistortion, EffectDelay, EffectReverb, EffectChorus, EffectBitcrusher, EffectFilter,
	EffectPhaser, EffectFlanger, EffectTremolo, EffectGate, EffectLimiter, EffectRingMod,
	EffectWaveshaper, EffectAutoWah, EffectCompressor, EffectTransient, EffectTape, EffectPitchShift,
}

// An unknown name is an error, never a slot that silently processes nothing.
func ParseEffectType(s string) (EffectType, error) {
	for _, t := range EffectTypes {
		if string(t) == s {
			return t, nil
		}
	}
	return "", fmt.Errorf("unknown insert effect type %q", s)
}

// A project or bundle naming an unknown effect is rejected at decode.
func (t *EffectType) UnmarshalText(b []byte) error {
	v, err := ParseEffectType(string(b))
	if err != nil {
		return err
	}
	*t = v
	return nil
}

// EffectSlot describes one effect in an instrument's insert chain.
type EffectSlot struct {
	Type    EffectType         `json:"type"`
	Enabled bool               `json:"enabled"`
	Params  map[string]float64 `json:"params,omitempty"`
}

// EffectParamDef describes a single parameter on an effect type.
type EffectParamDef struct {
	Name    string  `json:"name"`
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	Default float64 `json:"default"`
	Unit    string  `json:"unit,omitempty"` // "ms", "Hz", "dB", "%", ""
}

// InsertEffect extends Processor with reset and parameter control.
type InsertEffect interface {
	Processor
	Reset()
	SetParam(name string, value float64)
}

func InsertEffectCatalog() map[EffectType][]EffectParamDef {
	regs := EffectRegistrations()
	out := make(map[EffectType][]EffectParamDef, len(regs))
	for t, r := range regs {
		out[t] = r.Params
	}
	return out
}

// DefaultParams returns a copy of the default parameter map for a given effect type.
func DefaultParams(t EffectType) map[string]float64 {
	registryMu.RLock()
	reg, ok := registryMap[t]
	registryMu.RUnlock()
	if !ok {
		return nil
	}
	m := make(map[string]float64, len(reg.Params))
	for _, d := range reg.Params {
		m[d.Name] = d.Default
	}
	return m
}

func NewEffectProcessor(slot EffectSlot, sr int) InsertEffect {
	params := mergeDefaults(slot.Type, slot.Params)
	registryMu.RLock()
	reg, ok := registryMap[slot.Type]
	registryMu.RUnlock()
	if ok && reg.New != nil {
		return reg.New(sr, params)
	}
	return &passThrough{}
}

// mergeDefaults fills in missing parameters with defaults from the catalog.
func mergeDefaults(t EffectType, user map[string]float64) map[string]float64 {
	defs := DefaultParams(t)
	if defs == nil {
		defs = make(map[string]float64)
	}
	for k, v := range user {
		defs[k] = v
	}
	return defs
}

// passThrough is a no-op processor.
type passThrough struct{}

func (p *passThrough) ProcessSample(x float64) float64 { return x }
func (p *passThrough) Reset()                          {}
func (p *passThrough) SetParam(string, float64)        {}

// ProcessBlockBuf implements BlockProcessor for passThrough.
func (p *passThrough) ProcessBlockBuf(in, out []float32, samples int) {
	copy(out[:samples], in[:samples])
}
