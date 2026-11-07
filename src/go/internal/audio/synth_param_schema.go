package audio

import (
	"fmt"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// gen1_<f>..gen12_<f>.
const modularGenSlots = 12

// ABI order: the C struct field order and the JS MODULAR_PARAM_INDEX.
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

// Read only by the variant-8 kick voice.
func IsKickPhysicalField(field string) bool {
	return fieldInBlock("kickphys", field)
}

// Read only while the plate is running.
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

// Rows are never knobs, so these carry no ParamDef.
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

// Drives audio.js's write loop.
func ModularParamSchemaIndexMap() map[string]int {
	out := make(map[string]int, len(modularParamSchema))
	for i, name := range modularParamSchema {
		out[name] = i
	}
	return out
}

// The Init column: ModularSynthParamDefs' defaults and the registry's ValueDecl.Init.
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
