package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

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

// Derived from the registry (everything not Undoable), so it cannot drift.
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

// Each gap is an honest record: a document-scope kind that is not undoable says why.
func TestDocumentGapsAreReasoned(t *testing.T) {
	for _, a := range hooks.AllActions() {
		if a.Scope == hooks.ScopeDocument && !hooks.Undoable(a.Kind) {
			if a.ExcludedReason == "" {
				t.Errorf("document-scope gap %q must carry an ExcludedReason", a.Kind)
			}
		}
	}
}
