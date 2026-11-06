//go:build !test && !js

package audio

import (
	"reflect"
	"testing"
)

// TestP3_NoRenderFuncField locks the P3 invariant (config as single source of
// truth): the legacy per-instrument C-renderer dispatch field is retired. Every
// built-in instrument renders through the ONE modular pipeline (render_modular_p
// via renderModularBlock) — melodics via bakedModularRender/the recipe, drums via
// renderXVoice (the Phase-5 modular cutover). InstrumentConfig now carries only
// declarative render metadata (duration, amplitude); the "which C function"
// string is gone. Reintroducing it would re-open a bespoke/baked path that
// bypasses the pipeline, so this fails if the field comes back.
func TestP3_NoRenderFuncField(t *testing.T) {
	typ := reflect.TypeOf(InstrumentConfig{})
	for i := 0; i < typ.NumField(); i++ {
		if name := typ.Field(i).Name; name == "RenderFunc" {
			t.Error("InstrumentConfig.RenderFunc reintroduced — the legacy C-renderer dispatch field is retired (P3). Instruments must render via render_modular_p, not a per-instrument C function.")
		}
	}
}
