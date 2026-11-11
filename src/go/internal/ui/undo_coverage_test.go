package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// TestUndoRecordedSetMatchesDeclared asserts every undoable kind in the registry
// has a non-empty label in the derived documentScopeKinds, and no extra keys
// exist. (Focused label-presence guard; the deeper equality check is
// TestUndoableSetMatchesRegistry in undo_registry_drift_test.go.)
func TestUndoRecordedSetMatchesDeclared(t *testing.T) {
	for _, a := range hooks.AllActions() {
		if !hooks.Undoable(a.Kind) {
			continue
		}
		if documentScopeKinds[a.Kind] == "" {
			t.Errorf("undoable kind %q missing a label in documentScopeKinds", a.Kind)
		}
	}
	for k := range documentScopeKinds {
		if !hooks.Undoable(k) {
			t.Errorf("documentScopeKinds has %q but registry says not undoable", k)
		}
	}
}

// TestUndoRecordedSetExcludesNonDocument asserts kinds that emit events for
// coverage but cannot be snapshot-undone are NOT in the recorded-set. The
// excluded set is derived from the registry (everything not Undoable), so this
// can never drift from the classification.
func TestUndoRecordedSetExcludesNonDocument(t *testing.T) {
	for _, a := range hooks.AllActions() {
		if hooks.Undoable(a.Kind) {
			continue
		}
		if _, ok := documentScopeKinds[a.Kind]; ok {
			t.Errorf("%q is not undoable in the registry but appears in documentScopeKinds", a.Kind)
		}
	}
}

// TestDocumentGapsAreReasoned asserts every document-scope kind that is NOT
// undoable carries an ExcludedReason — the honest record of a known gap
// (e.g. row pan / sends are exported but have no UI commit site yet).
func TestDocumentGapsAreReasoned(t *testing.T) {
	for _, a := range hooks.AllActions() {
		if a.Scope == hooks.ScopeDocument && !hooks.Undoable(a.Kind) {
			if a.ExcludedReason == "" {
				t.Errorf("document-scope gap %q must carry an ExcludedReason", a.Kind)
			}
		}
	}
}
