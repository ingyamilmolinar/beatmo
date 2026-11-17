package audio

import (
	"math"
	"slices"
	"strings"
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// instrument_param_defs.go — the Synth tab's sub-project-1 adapter (spec §6):
// the UI keeps reading []ParamDef, now derived per instrument from its
// document. Replaced by module-grouped metadata in sub-project 3.

// instrumentParamDef is the generic ParamDef for name, its range widened to
// contain the instrument document's own value (a shipped value is never
// clamped away by its knob).
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

// InstrumentParamDefs is the instrument's knob set: every modular ParamDef in
// ABI order, Default = the document's value (FieldValues), ranges widened to
// contain it, plus the seeded-fundamental log range and the family ranges the
// recipe registry used to apply.
//
// READ-ONLY CONTRACT: the returned slice is cached per document revision and
// shared across callers and frames (the Synth tab asks every Layout) — never
// mutate it, exactly like the recipe registry's Params it replaces.
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
		// A table-gated stage (MODAL / STRIKE / NOISE) with an empty table has
		// no knobs: they would act on silence, and rows never come from the
		// Synth tab. Hidden, not dropped — the schema stays one ParamDef per
		// ABI field.
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
		// A seeded base pitch is the voice's own fundamental: a musical log
		// range instead of the engine's 0..20 kHz (the knob-range rule for a seeded base pitch).
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

// baseModularParamDefs is ModularSynthParamDefs built once (it is a pure
// function of the code, and building it allocates heavily).
func baseModularParamDefs() []ParamDef {
	baseDefsOnce.Do(func() { baseDefs = ModularSynthParamDefs() })
	return baseDefs
}

// InstrumentDeclaresField reports whether a flat field belongs to a module the
// instrument has (or could add) — FlatFieldDeclared over its flat view.
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

// tableGatedFields maps every value / switch of a table-gated stage to the row
// count that gates it.
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

// tableGatedStages are the stages that only mean something with table rows
// (the WAV-clone MODAL / NOISEBAND tables, and STRIKE, which reweights the
// MODAL rows): they are declared only when that table has rows — an empty one
// would offer knobs over silence, and rows are never added from the Synth tab.
var tableGatedStages = map[string]string{"modal": "modal_count", "strike": "modal_count", "noiseband": "noiseband_count"}

// FlatFieldDeclared is the Synth tab's ONE visibility rule (spec §6), read off
// an instrument's flat view (FieldValues): stage values and switches are always
// declared (an absent stage shows dimmed) — except a table-gated stage
// (tableGatedStages), declared only with rows; the oscillator switch and selector
// always; a source value only when the source type osc_type selects declares
// it; a gen1_ field only when the layer type gen1_source (+ its variant
// constants) selects declares it. Layer type constants (kick_enabled …) never;
// slots 2..12 never. It replaces the family / oscillator-model / cymbal-modal /
// kick-physical hide predicates, which encoded the same facts by hand.
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
					// A view without the derived row count (a schema's
					// Defaults): InstrumentParamDefs already hid the stage's
					// knobs when its table is empty.
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
