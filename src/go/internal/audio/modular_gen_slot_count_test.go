//go:build !test && !js

package audio

import (
	"reflect"
	"strings"
	"testing"
)

// TestModularGenSlotCountIsSingleSourceOfTruth pins modularGenSlots as the lone
// Go-side definition of the gen-bank slot count. Before this guard the literal
// 12 was duplicated across the ModularParams [12] array declarations, the
// per-family binding clear-loops (for i := 1; i < 12), and the schema/recipe
// loops — any one of which could silently drift. The test enforces:
//
//	(1) modularGenSlots equals the C ABI (modular.h: float gen_*[12]).
//	(2) every Gen* fixed-size array in ModularParams is sized by that constant.
//
// Lives under !test (cgo desktop build) because ModularParams is the real cgo
// struct; the fast -tags test path stubs the engine and never sees it.
func TestModularGenSlotCountIsSingleSourceOfTruth(t *testing.T) {
	// (1) Go↔C ABI contract: modular.h declares every gen-bank field as [12].
	const cABIGenSlots = 12
	if modularGenSlots != cABIGenSlots {
		t.Fatalf("modularGenSlots=%d != C ABI gen_*[%d] — Go/C gen-bank ABI drift; update both modular.h and the const together", modularGenSlots, cABIGenSlots)
	}

	// (2) Every Gen* array field must be exactly modularGenSlots long.
	tp := reflect.TypeOf(ModularParams{})
	checked := 0
	for i := 0; i < tp.NumField(); i++ {
		f := tp.Field(i)
		if !strings.HasPrefix(f.Name, "Gen") || f.Type.Kind() != reflect.Array {
			continue
		}
		checked++
		if f.Type.Len() != modularGenSlots {
			t.Errorf("ModularParams.%s has length %d, want modularGenSlots=%d (stray literal?)", f.Name, f.Type.Len(), modularGenSlots)
		}
	}
	if checked == 0 {
		t.Fatal("no Gen* [N]float64 arrays found in ModularParams — test wiring is stale")
	}
}
