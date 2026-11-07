//go:build !js

package main

import (
	"reflect"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

func refServing(id string, insts ...instrument.ID) wavref.Ref {
	return wavref.Ref{
		ID: id, Tier: wavref.TierInstrument, Instruments: insts,
		File:   id + ".wav",
		Source: &wavref.Source{Provider: "local", URL: "http://example/" + id},
	}
}

func refByID(t *testing.T, refs []wavref.Ref, id string) wavref.Ref {
	t.Helper()
	for _, r := range refs {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("ref %q not in result: %v", id, refIDs(refs))
	return wavref.Ref{}
}

func refIDs(refs []wavref.Ref) []string {
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, r.ID)
	}
	return out
}

// The core contract: after re-pointing, exactly one ref is ground truth for
// the instrument.
func TestRepointInstrument_LeavesOnlyTheWinnerServingTheInstrument(t *testing.T) {
	refs := []wavref.Ref{
		refServing("violin-old", roster.Violin, roster.ViolinEnsemble),
		refServing("violin-new", roster.Violin),
	}

	got := repointInstrument(refs, roster.Violin, "violin-new", "superseded")

	if serves := refByID(t, got, "violin-old").Instruments; !reflect.DeepEqual(serves, []instrument.ID{roster.ViolinEnsemble}) {
		t.Errorf("loser still serves %v, want only [violin-ensemble]", serves)
	}
	if serves := refByID(t, got, "violin-new").Instruments; !reflect.DeepEqual(serves, []instrument.ID{roster.Violin}) {
		t.Errorf("winner serves %v, want [violin]", serves)
	}
}

// Re-pointing must be reversible: the displaced reference survives on the
// winner as an alternative, with a reason, so the decision can be revisited
// when an instrument proves hard to tune.
func TestRepointInstrument_RecordsTheLoserAsAnAlternativeOnTheWinner(t *testing.T) {
	refs := []wavref.Ref{
		refServing("violin-old", roster.Violin, roster.ViolinEnsemble),
		refServing("violin-new", roster.Violin),
	}

	got := repointInstrument(refs, roster.Violin, "violin-new", "adopted a local file")

	alts := refByID(t, got, "violin-new").Alternatives
	if len(alts) != 1 {
		t.Fatalf("winner carries %d alternatives, want 1: %+v", len(alts), alts)
	}
	if alts[0].URL != "http://example/violin-old" {
		t.Errorf("alternative URL = %q, want the loser's source URL", alts[0].URL)
	}
	if alts[0].Reason != "adopted a local file" {
		t.Errorf("alternative Reason = %q, want the caller's reason", alts[0].Reason)
	}
}

// A reference left serving nothing is dead weight that would fail
// Cat.Validate on the next load, so it is dropped.
func TestRepointInstrument_DropsARefLeftServingNothing(t *testing.T) {
	refs := []wavref.Ref{
		refServing("violin-old", roster.Violin),
		refServing("violin-new", roster.Violin),
	}

	got := repointInstrument(refs, roster.Violin, "violin-new", "superseded")

	if len(got) != 1 || got[0].ID != "violin-new" {
		t.Errorf("result = %v, want only [violin-new]", refIDs(got))
	}
}

// Siblings are not collateral damage: re-pointing "violin" must leave the
// reference for every OTHER instrument exactly as it was.
func TestRepointInstrument_LeavesUnrelatedRefsUntouched(t *testing.T) {
	cello := refServing("cello-C2", roster.Cello)
	refs := []wavref.Ref{cello, refServing("violin-old", roster.Violin), refServing("violin-new", roster.Violin)}

	got := repointInstrument(refs, roster.Violin, "violin-new", "superseded")

	if !reflect.DeepEqual(refByID(t, got, "cello-C2"), cello) {
		t.Errorf("unrelated ref changed:\n got %+v\nwant %+v", refByID(t, got, "cello-C2"), cello)
	}
}

// Status refs record "no acoustic counterpart"; they are not candidates.
func TestRepointInstrument_IgnoresStatusRefs(t *testing.T) {
	status := wavref.Ref{ID: "synthetic-violin", Tier: wavref.TierInstrument,
		Instruments: []instrument.ID{roster.Violin}, Status: wavref.StatusSynthetic, Note: "n/a"}
	refs := []wavref.Ref{status, refServing("violin-new", roster.Violin)}

	got := repointInstrument(refs, roster.Violin, "violin-new", "superseded")

	if !reflect.DeepEqual(refByID(t, got, "synthetic-violin"), status) {
		t.Errorf("status ref changed:\n got %+v\nwant %+v", refByID(t, got, "synthetic-violin"), status)
	}
}

// The caller's slice can alias the memoized wavref.Catalogue() singleton's
// backing arrays, so re-pointing must not write through them -- a mutation
// here would corrupt every later Catalogue() call in the same process.
func TestRepointInstrument_DoesNotMutateTheInputSlice(t *testing.T) {
	refs := []wavref.Ref{
		refServing("violin-old", roster.Violin, roster.ViolinEnsemble),
		refServing("violin-new", roster.Violin),
	}
	before := []instrument.ID{roster.Violin, roster.ViolinEnsemble}

	repointInstrument(refs, roster.Violin, "violin-new", "superseded")

	if !reflect.DeepEqual(refs[0].Instruments, before) {
		t.Errorf("input ref mutated: got %v, want unchanged %v", refs[0].Instruments, before)
	}
}
