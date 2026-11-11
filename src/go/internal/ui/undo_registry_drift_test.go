package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// TestUndoableSetMatchesRegistry asserts the derived undo recorded-set is
// exactly the registry's undoable subset — the single-source-of-truth invariant.
func TestUndoableSetMatchesRegistry(t *testing.T) {
	want := map[hooks.Kind]string{}
	for _, a := range hooks.AllActions() {
		if hooks.Undoable(a.Kind) {
			want[a.Kind] = a.Label
		}
	}
	if len(want) != len(documentScopeKinds) {
		t.Fatalf("documentScopeKinds size=%d, registry undoable size=%d", len(documentScopeKinds), len(want))
	}
	for k, label := range want {
		got, ok := documentScopeKinds[k]
		if !ok {
			t.Errorf("registry says %q is undoable but documentScopeKinds lacks it", k)
			continue
		}
		if got != label {
			t.Errorf("label drift for %q: documentScopeKinds=%q registry=%q", k, got, label)
		}
	}
	for k := range documentScopeKinds {
		if !hooks.Undoable(k) {
			t.Errorf("documentScopeKinds has %q but registry says it is not undoable", k)
		}
	}
}
