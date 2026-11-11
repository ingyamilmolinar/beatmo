package hooks

import (
	"strings"
	"testing"
)

// TestEveryKindClassified asserts every Kind in KindAll has exactly one
// ActionRegistry entry and vice versa. This is the central completeness guard:
// a new Kind without a registry entry (or a stale entry) trips it.
func TestEveryKindClassified(t *testing.T) {
	inReg := map[Kind]int{}
	for _, a := range ActionRegistry {
		inReg[a.Kind]++
	}
	for _, k := range KindAll {
		switch inReg[k] {
		case 0:
			t.Errorf("Kind %q has no ActionRegistry entry — classify it in actions.go", k)
		case 1:
			// ok
		default:
			t.Errorf("Kind %q has %d ActionRegistry entries — must be exactly one", k, inReg[k])
		}
	}
	known := map[Kind]bool{}
	for _, k := range KindAll {
		known[k] = true
	}
	for _, a := range ActionRegistry {
		if !known[a.Kind] {
			t.Errorf("ActionRegistry has %q which is not in KindAll", a.Kind)
		}
	}
}

// TestRegistryNoDuplicateKinds guards against copy-paste duplicates.
func TestRegistryNoDuplicateKinds(t *testing.T) {
	seen := map[Kind]bool{}
	for _, a := range ActionRegistry {
		if seen[a.Kind] {
			t.Errorf("duplicate ActionRegistry entry for %q", a.Kind)
		}
		seen[a.Kind] = true
	}
}

// TestExcludedReasonOnlyOnDocument asserts ExcludedReason is only set on
// ScopeDocument entries (it means "document state, deliberately not undoable
// yet" — meaningless on non-document scopes, which are never undoable).
func TestExcludedReasonOnlyOnDocument(t *testing.T) {
	for _, a := range ActionRegistry {
		if a.ExcludedReason != "" && a.Scope != ScopeDocument {
			t.Errorf("%q has ExcludedReason on non-document scope %v", a.Kind, a.Scope)
		}
	}
}

// TestUndoableImpliesDocument asserts the Undoable helper only ever returns
// true for document-scope, non-excluded entries.
func TestUndoableImpliesDocument(t *testing.T) {
	for _, a := range ActionRegistry {
		if Undoable(a.Kind) {
			if a.Scope != ScopeDocument || a.ExcludedReason != "" {
				t.Errorf("Undoable(%q)=true but scope=%v reason=%q", a.Kind, a.Scope, a.ExcludedReason)
			}
		}
	}
}

// TestEveryActionHasLowercaseTag asserts every registry entry carries a
// non-empty, all-lowercase bracket tag (the single source of truth for the
// INFO tag column).
func TestEveryActionHasLowercaseTag(t *testing.T) {
	for _, a := range ActionRegistry {
		if a.Tag == "" {
			t.Errorf("Kind %q has empty Tag", a.Kind)
			continue
		}
		if a.Tag != strings.ToLower(a.Tag) {
			t.Errorf("Kind %q tag %q is not lowercase", a.Kind, a.Tag)
		}
	}
}

func TestInteractionScopeNotUndoable(t *testing.T) {
	m := ActionMeta{Kind: "x.test", Scope: ScopeInteraction}
	if m.Scope == ScopeDocument {
		t.Fatal("ScopeInteraction must not equal ScopeDocument")
	}
}
