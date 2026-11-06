//go:build !js

package main

import (
	"path/filepath"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// verdict_supersede_test.go — re-auditioning a family must REPLACE its verdict,
// not append a second one.
//
// Re-auditing is a first-class workflow, not an edge case: any change to the
// render or level-match pipeline invalidates the affected families' recorded
// judgements, and the fix is to listen again. A blind append leaves two rows
// for one family in a file that is the labelled ground truth for downstream
// metric validation, with nothing but line order to say which is current.

func TestRecordVerdict_AppendsWhenNew(t *testing.T) {
	p := filepath.Join(t.TempDir(), "v.jsonl")
	if err := recordVerdict(p, earVerdict{Family: roster.Cello, Verdict: "n"}); err != nil {
		t.Fatal(err)
	}
	if err := recordVerdict(p, earVerdict{Family: roster.Oboe, Verdict: "p"}); err != nil {
		t.Fatal(err)
	}
	got, err := readVerdicts(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d verdicts, want 2", len(got))
	}
	if got[0].Family != roster.Cello || got[1].Family != roster.Oboe {
		t.Errorf("append order not preserved: %v", got)
	}
}

func TestRecordVerdict_SupersedesInPlace(t *testing.T) {
	p := filepath.Join(t.TempDir(), "v.jsonl")
	for _, v := range []earVerdict{
		{Family: roster.Cello, Verdict: "n", Note: "too buzzy", Tags: []string{"noisy"}},
		{Family: roster.Oboe, Verdict: "p"},
		{Family: roster.Violin, Verdict: "1"},
	} {
		if err := recordVerdict(p, v); err != nil {
			t.Fatal(err)
		}
	}

	// Same verdict again: the prose still describes this judgement, so it
	// carries forward.
	if err := recordVerdict(p, earVerdict{Family: roster.Cello, Verdict: "n", Reference: "cello-A3"}); err != nil {
		t.Fatal(err)
	}
	got, err := readVerdicts(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("re-recording appended a duplicate: got %d rows, want 3: %v", len(got), got)
	}
	if got[0].Family != roster.Cello {
		t.Errorf("superseding must keep the row's position, got order %v", []string{string(got[0].Family), string(got[1].Family), string(got[2].Family)})
	}
	if got[0].Reference != "cello-A3" {
		t.Errorf("the new verdict's fields must win, got Reference=%q", got[0].Reference)
	}
	if got[0].Note != "too buzzy" {
		t.Errorf("an unchanged verdict must keep its note, got %q", got[0].Note)
	}

	// Changed verdict: the old "why" describes an answer that no longer
	// stands, so it must NOT be carried forward as if it still applied.
	if err := recordVerdict(p, earVerdict{Family: roster.Cello, Verdict: "y"}); err != nil {
		t.Fatal(err)
	}
	got, _ = readVerdicts(p)
	if len(got) != 3 {
		t.Fatalf("got %d rows, want 3", len(got))
	}
	if got[0].Verdict != "y" {
		t.Errorf("verdict not updated: %q", got[0].Verdict)
	}
	if got[0].Note != "" || len(got[0].Tags) != 0 {
		t.Errorf("a CHANGED verdict must drop the stale note/tags, got note=%q tags=%v",
			got[0].Note, got[0].Tags)
	}
}

// A note supplied with the new verdict always wins over the carried-forward one.
func TestRecordVerdict_NewNoteWins(t *testing.T) {
	p := filepath.Join(t.TempDir(), "v.jsonl")
	if err := recordVerdict(p, earVerdict{Family: roster.Cello, Verdict: "n", Note: "old"}); err != nil {
		t.Fatal(err)
	}
	if err := recordVerdict(p, earVerdict{Family: roster.Cello, Verdict: "n", Note: "new"}); err != nil {
		t.Fatal(err)
	}
	got, _ := readVerdicts(p)
	if len(got) != 1 || got[0].Note != "new" {
		t.Errorf("got %v, want a single row noted %q", got, "new")
	}
}
