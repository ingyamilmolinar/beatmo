package audio

import (
	"math"
	"slices"
	"strings"
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Widened to contain the document's own value, so a shipped value is never clamped away by its knob.
func instrumentParamDef(instrumentID instrument.ID, name string) ParamDef {
	d, ok := modularSchemaDefIndex[name]
	if !ok {
		return ParamDef{}
	}
	if v, ok := MergedInstrumentParamsRO(instrumentID)[name]; ok {
		d.Min = math.Min(d.Min, v)
		d.Max = math.Max(d.Max, v)
	}
	return d
}

// READ-ONLY: cached per document revision and shared across callers and frames (the Synth tab asks every Layout).
func InstrumentParamDefs(instrumentID instrument.ID) []ParamDef {
	if !HasInstrumentDoc(instrumentID) {
		if _, ok := EnsureInstrumentDoc(instrumentID); !ok {
			return nil
		}
	}
	rev := InstrumentDocRev(instrumentID)
	paramDefsMu.Lock()
	if e, ok := paramDefsCache[instrumentID]; ok && e.rev == rev {
		paramDefsMu.Unlock()
		return e.defs
	}
	paramDefsMu.Unlock()
	vals := MergedInstrumentParamsRO(instrumentID)
	base := baseModularParamDefs()
	defs := make([]ParamDef, len(base))
	copy(defs, base)
	for i := range defs {
		// Knobs over an empty table would act on silence, and rows never come from the Synth tab. Hidden, not dropped: one
		// ParamDef per ABI field stays the schema.
		if count, gated := tableGatedFields()[defs[i].Name]; gated && vals[count] <= 0 {
			defs[i].Group = SynthHiddenGroup
		}
	}
	for i := range defs {
		v, ok := vals[defs[i].Name]
		if !ok {
			continue
		}
		defs[i].Default = v
		defs[i].Min = math.Min(defs[i].Min, v)
		defs[i].Max = math.Max(defs[i].Max, v)
		// A seeded base pitch gets a musical log range instead of the engine's 0..20 kHz.
		if defs[i].Name == "voice_freq_hz" && v > 0 {
			defs[i].Min = math.Min(VoiceFreqKnobMin, v)
			defs[i].Max = math.Max(VoiceFreqKnobMax, v)
			defs[i].Curve = "log"
		}
	}
	applyFamilyKnobRanges(defs, vals)
	paramDefsMu.Lock()
	paramDefsCache[instrumentID] = paramDefsEntry{rev: rev, defs: defs}
	paramDefsMu.Unlock()
	return defs
}

type paramDefsEntry struct {
	rev  uint64
	defs []ParamDef
}

var (
	paramDefsMu    sync.Mutex
	paramDefsCache = map[instrument.ID]paramDefsEntry{}

	baseDefsOnce sync.Once
	baseDefs     []ParamDef
)

// A pure function of the code that allocates heavily, so built once.
func baseModularParamDefs() []ParamDef {
	baseDefsOnce.Do(func() { baseDefs = ModularSynthParamDefs() })
	return baseDefs
}

func InstrumentDeclaresField(instrumentID instrument.ID, name string) bool {
	if _, ok := InstrumentDoc(instrumentID); !ok {
		return false
	}
	return FlatFieldDeclared(MergedInstrumentParamsRO(instrumentID), name)
}

var (
	tableGatedOnce sync.Once
	tableGatedMap  map[string]string
)

func tableGatedFields() map[string]string {
	tableGatedOnce.Do(func() {
		tableGatedMap = map[string]string{}
		for _, t := range BuiltinModuleRegistry().Types() {
			count, ok := tableGatedStages[t.Type]
			if !ok {
				continue
			}
			for _, v := range t.Values {
				tableGatedMap[v.Name] = count
			}
			if t.Enable != nil {
				tableGatedMap[t.Enable.Field] = count
			}
		}
	})
	return tableGatedMap
}

// STRIKE reweights the MODAL rows. Declared only with rows: an empty table would offer knobs over silence.
var tableGatedStages = map[string]string{"modal": "modal_count", "strike": "modal_count", "noiseband": "noiseband_count"}

// The Synth tab's ONE visibility rule, read off the flat view: stage values always (an absent stage shows dimmed) except
// table-gated stages without rows; a source or gen1_ field only when the selected type declares it; type constants
// and slots 2..12 never.
func FlatFieldDeclared(vals map[string]float64, name string) bool {
	reg := BuiltinModuleRegistry()
	declares := func(t *instrument.ModuleTypeDecl, field string) bool {
		for _, v := range t.Values {
			if v.Name == field {
				return true
			}
		}
		for _, s := range t.Set {
			if s.Field == field {
				return true
			}
		}
		return t.Enable != nil && t.Enable.Field == field
	}
	if rest, ok := strings.CutPrefix(name, "gen1_"); ok {
		src := vals["gen1_source"]
		for _, t := range reg.Types() {
			if t.Kind != instrument.KindLayer || t.Enable.Value != src {
				continue
			}
			match := true
			for _, s := range t.Set {
				if vals[instrument.LayerField(0, s.Field)] != s.Value {
					match = false
				}
			}
			if match {
				return declares(t, rest)
			}
		}
		return false
	}
	if strings.HasPrefix(name, "gen") {
		return false
	}
	var src *instrument.ModuleTypeDecl
	for _, t := range reg.Types() {
		switch t.Kind {
		case instrument.KindStage:
			if declares(t, name) {
				if count, gated := tableGatedStages[t.Type]; gated {
					if c, known := vals[count]; known {
						return c > 0
					}
					// A view without the derived row count: InstrumentParamDefs already hid an empty table's knobs.
				}
				return true
			}
		case instrument.KindSource:
			if name == "osc_type" || (t.Enable != nil && t.Enable.Field == name) {
				return true
			}
			for _, s := range t.Set {
				if s.Field == "osc_type" && s.Value == vals["osc_type"] {
					src = t
				}
			}
			for _, v := range t.Values {
				if v.Name == "osc_type" && slices.Contains(v.Allowed, vals["osc_type"]) {
					src = t
				}
			}
		}
	}
	return src != nil && declares(src, name)
}
