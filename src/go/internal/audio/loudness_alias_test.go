//go:build !js

package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// variants.go used to strip a trailing "-1"/"-2" off v.Name before
// normalising. After the Phase 0 rename no ROSTER id ends that way, so the
// strip could no longer fire for any registered instrument -- which is what
// this test proves. It does NOT cover runtime instance ids (e.g. "violin-2",
// "piano-grand-3"): those DO end in an ordinal, and they do reach
// renderAndCache/renderParamAndCache. The strip is still safe to delete for
// them too, but for a different reason -- registerInstanceAlias
// (instance_instrument_notjs.go) makes an instance id share the BASE
// instrument's already-registered Instrument value via Register(id, inst), so
// the instrument that ends up rendering always carries the base's name
// ("violin"), never the instance id's own ("violin-2"), so the renderer never
// sees an instance id at all, ordinal or not.
// This test guards the roster half of that claim, and fails if a future
// variant reintroduces an ordinal ROSTER name.
func TestNoVariantNameEndsInAnOrdinal(t *testing.T) {
	for _, id := range roster.All {
		if len(id) > 2 && id[len(id)-2] == '-' {
			if c := id[len(id)-1]; c >= '1' && c <= '9' {
				t.Errorf("%s ends in an ordinal suffix; Phase 0 removed those", id)
			}
		}
	}
}
