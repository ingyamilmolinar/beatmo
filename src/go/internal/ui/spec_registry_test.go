package ui

// A spec naming an anchor nobody registered would fail only the first time that component drew.

import (
	"sort"
	"testing"
)

// Bindings are installed by init() in the owning files. Vacuous while no spec declares an anchor; the first
// `dynamic:` or `animation:` block makes it load-bearing.
func TestAnchorsAllRegistered(t *testing.T) {
	bound := allBoundAnchors()
	var missing []string

	for id := ComponentID(0); id < componentCount; id++ {
		spec := componentSpecs[id]
		if name := spec.DynamicAnchor; name != "" && !bound[name] {
			missing = append(missing, "dynamic anchor "+name+" (component "+spec.ID.String()+") has no BindDynamic registration")
		}
		if name := spec.AnimationAnchor; name != "" && !bound[name] {
			missing = append(missing, "animation anchor "+name+" (component "+spec.ID.String()+") has no BindAnimation registration")
		}
	}

	if len(missing) == 0 {
		return
	}
	sort.Strings(missing)
	for _, m := range missing {
		t.Error(m)
	}
}

func (id ComponentID) String() string { return "ComponentID(" + itoa(int(id)) + ")" }
