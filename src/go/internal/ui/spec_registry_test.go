package ui

// spec_registry_test.go — Closes the audit gap on dynamic / animation
// anchors. The check here is "every anchor named in DESIGN.md has a
// runtime binding before the first frame is drawn." Without it the
// generator could happily emit a spec whose DynamicAnchor field
// references a function nobody registered, and the bug would show up
// only the first time someone drew that component.

import (
	"sort"
	"testing"
)

// TestAnchorsAllRegistered scans every generated ComponentSpec for
// dynamic / animation anchors, then asserts every named anchor has a
// runtime closure registered via BindDynamic / BindAnimation. The
// bindings are expected to be installed by package init() in the
// owning files (e.g. transport_zone.go for RecordPulseAnimator,
// row_rack_zone.go for ColorSwatchColorFn).
//
// While the codebase is mid-migration this test will pass vacuously
// (no specs declare anchors yet). The first DESIGN.md change that
// adds a `dynamic:` or `animation:` block makes the test load-
// bearing — exactly when it needs to be.
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

// String returns the ComponentID's stable name for diagnostics. The
// generator could add this to design_components.gen.go in a future
// PR; for now the basic numeric repr is enough for the failure
// message above.
func (id ComponentID) String() string { return "ComponentID(" + itoa(int(id)) + ")" }
