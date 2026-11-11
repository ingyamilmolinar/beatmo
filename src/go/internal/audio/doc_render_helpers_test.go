//go:build !test && !js

package audio

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// renderFieldsForTest renders id's document with fields set through it — RAW
// C output, n samples at sr (RenderInstrumentWithFields). The test-side form of
// "render this instrument with these knob values".
func renderFieldsForTest(t testing.TB, id instrument.ID, sr, n int, fields map[string]float64) []float32 {
	t.Helper()
	buf, err := RenderInstrumentWithFields(id, sr, n, fields)
	if err != nil {
		t.Fatalf("render %s: %v", id, err)
	}
	return buf
}

// fieldsOfForTest is a copy of id's flat field view (instrument.FieldValues of
// its document): what used to be "the recipe's merged params".
func fieldsOfForTest(id string) map[string]float64 {
	src := MergedInstrumentParamsRO(instrument.ID(id))
	out := make(map[string]float64, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

// initBlockForTest is the engine-level tests' neutral voice: every field at its
// creation value (the modularFields Init column) overlaid by fields. It builds
// a raw engine block for tests of the C engine itself; product voices come from
// documents (Flatten).
func initBlockForTest(fields map[string]float64) modularBlock {
	inits := modularFieldInits()
	b := make(modularBlock, len(modularParamSchema))
	for i, name := range modularParamSchema {
		if v, ok := fields[name]; ok {
			b[i] = v
		} else {
			b[i] = inits[name]
		}
	}
	return b
}

func ms(d time.Duration) float64 { return float64(d.Nanoseconds()) / 1e6 }
