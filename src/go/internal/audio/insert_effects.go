package audio

// EffectType identifies a kind of insert effect.
type EffectType string

const (
	EffectDistortion EffectType = "distortion"
	EffectDelay      EffectType = "delay"
	EffectReverb     EffectType = "reverb"
	EffectChorus     EffectType = "chorus"
	EffectBitcrusher EffectType = "bitcrusher"
	EffectFilter     EffectType = "filter"
)

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

// InsertEffectCatalog returns parameter definitions for all available effect types.
func InsertEffectCatalog() map[EffectType][]EffectParamDef {
	return map[EffectType][]EffectParamDef{
		EffectDistortion: {
			{Name: "drive", Min: 1, Max: 20, Default: 2, Unit: ""},
			{Name: "tone", Min: 200, Max: 8000, Default: 4000, Unit: "Hz"},
			{Name: "mix", Min: 0, Max: 1, Default: 1, Unit: ""},
		},
		EffectDelay: {
			{Name: "time", Min: 10, Max: 1000, Default: 250, Unit: "ms"},
			{Name: "feedback", Min: 0, Max: 0.95, Default: 0.4, Unit: ""},
			{Name: "mix", Min: 0, Max: 1, Default: 0.3, Unit: ""},
		},
		EffectReverb: {
			{Name: "room", Min: 0, Max: 1, Default: 0.5, Unit: ""},
			{Name: "damping", Min: 0, Max: 1, Default: 0.5, Unit: ""},
			{Name: "mix", Min: 0, Max: 1, Default: 0.3, Unit: ""},
		},
		EffectChorus: {
			{Name: "rate", Min: 0.1, Max: 10, Default: 1.5, Unit: "Hz"},
			{Name: "depth", Min: 0, Max: 20, Default: 5, Unit: "ms"},
			{Name: "mix", Min: 0, Max: 1, Default: 0.5, Unit: ""},
		},
		EffectBitcrusher: {
			{Name: "bits", Min: 2, Max: 16, Default: 8, Unit: ""},
			{Name: "rate", Min: 0.01, Max: 1, Default: 0.5, Unit: ""},
			{Name: "mix", Min: 0, Max: 1, Default: 0.5, Unit: ""},
		},
		EffectFilter: {
			{Name: "mode", Min: 0, Max: 2, Default: 0, Unit: ""}, // 0=LP, 1=HP, 2=BP
			{Name: "cutoff", Min: 20, Max: 20000, Default: 1000, Unit: "Hz"},
			{Name: "q", Min: 0.1, Max: 10, Default: 0.707, Unit: ""},
			{Name: "mix", Min: 0, Max: 1, Default: 1, Unit: ""},
		},
	}
}

// DefaultParams returns a copy of the default parameter map for a given effect type.
func DefaultParams(t EffectType) map[string]float64 {
	cat := InsertEffectCatalog()
	defs, ok := cat[t]
	if !ok {
		return nil
	}
	m := make(map[string]float64, len(defs))
	for _, d := range defs {
		m[d.Name] = d.Default
	}
	return m
}

// NewEffectProcessor creates a Processor for the given EffectSlot.
// If the slot is disabled, it returns a pass-through processor.
func NewEffectProcessor(slot EffectSlot, sr int) InsertEffect {
	params := mergeDefaults(slot.Type, slot.Params)
	switch slot.Type {
	case EffectDistortion:
		return newDistortion(sr, params)
	case EffectDelay:
		return newDelay(sr, params)
	case EffectReverb:
		return newReverb(sr, params)
	case EffectChorus:
		return newChorus(sr, params)
	case EffectBitcrusher:
		return newBitcrusher(sr, params)
	case EffectFilter:
		return newFilter(sr, params)
	default:
		return &passThrough{}
	}
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
