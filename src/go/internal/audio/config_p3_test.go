//go:build !test && !js

package audio

import (
	"reflect"
	"testing"
)

// Every built-in renders through the one modular pipeline: a "which C function" field would reopen a bespoke path
// that bypasses it.
func TestP3_NoRenderFuncField(t *testing.T) {
	typ := reflect.TypeOf(InstrumentConfig{})
	for i := 0; i < typ.NumField(); i++ {
		if name := typ.Field(i).Name; name == "RenderFunc" {
			t.Error("InstrumentConfig.RenderFunc reintroduced — the legacy C-renderer dispatch field is retired (P3). Instruments must render via render_modular_p, not a per-instrument C function.")
		}
	}
}
