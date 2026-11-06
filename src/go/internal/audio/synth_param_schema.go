package audio

import (
	"fmt"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// modularGenSlots is the slot count of the generic gen-bank fields (source,
// wave, freq, …): gen1_<f>..gen12_<f>.
const modularGenSlots = 12

// modularParamSchema is the flat param-name list of the modular block in ABI
// order (== the C struct field order, == the JS MODULAR_PARAM_INDEX), derived
// from modularFields.
var modularParamSchema = func() []string {
	var s []string
	for _, f := range modularFields {
		if f.Rows > 0 {
			for i := 0; i < f.Rows; i++ {
				s = append(s, instrument.TableCell(f.Name, i))
			}
			continue
		}
		if f.Slots == 0 {
			s = append(s, f.Name)
			continue
		}
		for k := 1; k <= f.Slots; k++ {
			s = append(s, fmt.Sprintf("gen%d_%s", k, f.Name))
		}
	}
	return s
}()

// IsKickPhysicalField reports whether a gen-slot field name (without its
// gen<N>_ prefix) is one of the physical-kick columns — knobs only the
// variant-8 (Physical) kick voice reads.
func IsKickPhysicalField(field string) bool {
	return fieldInBlock("kickphys", field)
}

// IsCymbalModalField reports whether a gen-slot field name (without its
// gen<N>_ prefix) is one of the cymbal modal-plate columns — knobs the cymbal
// voice reads only when its plate is running.
func IsCymbalModalField(field string) bool {
	return fieldInBlock("cymmodal", field)
}

func fieldInBlock(block, field string) bool {
	for _, f := range modularFields {
		if f.Block == block && f.Name == field {
			return true
		}
	}
	return false
}

// IsTableField reports whether a schema field is written by Flatten from a
// stage table — a cell of a table column, or a table's row count. Such fields
// carry no ParamDef: rows are never knobs.
func IsTableField(name string) bool { return tableFields[name] }

var tableFields = func() map[string]bool {
	m := map[string]bool{}
	for _, f := range modularFields {
		switch {
		case f.Rows > 0:
			for i := 0; i < f.Rows; i++ {
				m[instrument.TableCell(f.Name, i)] = true
			}
		case f.Derived:
			m[f.Name] = true
		}
	}
	return m
}()

// ModularParamSchema returns the modular block's param names in ABI order.
func ModularParamSchema() []string {
	out := make([]string, len(modularParamSchema))
	copy(out, modularParamSchema)
	return out
}

// ModularParamSchemaIndexMap returns paramName → struct index for the modular
// block (audio.js write-loop driver).
func ModularParamSchemaIndexMap() map[string]int {
	out := make(map[string]int, len(modularParamSchema))
	for i, name := range modularParamSchema {
		out[name] = i
	}
	return out
}

// modularFieldInits is each modular field's creation value (the modularFields
// Init column) — ModularSynthParamDefs' defaults, and through them the module
// registry's ValueDecl.Init.
func modularFieldInits() map[string]float64 {
	out := make(map[string]float64, len(modularParamSchema))
	for _, f := range modularFields {
		if f.Rows > 0 {
			for i := 0; i < f.Rows; i++ {
				out[instrument.TableCell(f.Name, i)] = f.Init
			}
			continue
		}
		if f.Slots == 0 {
			out[f.Name] = f.Init
			continue
		}
		for k := 1; k <= f.Slots; k++ {
			out[fmt.Sprintf("gen%d_%s", k, f.Name)] = f.Init
		}
	}
	return out
}
