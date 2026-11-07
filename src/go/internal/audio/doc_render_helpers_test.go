//go:build !test && !js

package audio

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// RAW C output.
func renderFieldsForTest(t testing.TB, id instrument.ID, sr, n int, fields map[string]float64) []float32 {
	t.Helper()
	buf, err := RenderInstrumentWithFields(id, sr, n, fields)
	if err != nil {
		t.Fatalf("render %s: %v", id, err)
	}
	return buf
}

func fieldsOfForTest(id string) map[string]float64 {
	src := MergedInstrumentParamsRO(instrument.ID(id))
	out := make(map[string]float64, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

// Every field at its Init value overlaid by fields: a raw block for tests of the C engine itself; product voices
// come from documents.
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
