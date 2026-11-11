//go:build !js

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func TestSessionRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.jsonl")
	s := earSession{Session: "2026-09-04T18:00:00Z", Family: roster.Oboe, Reference: "oboe-A4", SemisFromA3: 12, DurSec: 2,
		Current: sessionClip{ID: "oboe", RecipeSHA: "aa"},
		Candidates: []sessionCandidate{
			{sessionClip: sessionClip{ID: "A", RecipeSHA: "bb", Overrides: map[string]float64{"filter_cutoff": 5000}}, Label: "darker", Rating: 4, VsCurrent: "better",
				Scales: []sessionScale{{Q: "breathiness", Axis: "noise.persistence", Answer: -1}}, Adjectives: []string{"nasal"}},
			{sessionClip: sessionClip{ID: "B", RecipeSHA: "cc"}, Label: "formant", Rating: 2, VsCurrent: "worse"},
		}, Note: "A is close"}
	if err := appendSession(p, s); err != nil {
		t.Fatal(err)
	}
	got, err := readSessions(p)
	if err != nil || len(got) != 1 || got[0].Candidates[0].Overrides["filter_cutoff"] != 5000 || got[0].Candidates[0].Scales[0].Answer != -1 {
		t.Fatalf("round trip failed: %+v err=%v", got, err)
	}
	if _, err := readSessions(filepath.Join(t.TempDir(), "missing.jsonl")); err != nil {
		t.Fatalf("missing file must be nil,nil: %v", err)
	}
	_ = os.Remove(p)
}

func TestPrefPairsFromSessions(t *testing.T) {
	ss := []earSession{{Family: roster.Oboe, Reference: "oboe-A4", Current: sessionClip{ID: "oboe"},
		Candidates: []sessionCandidate{
			{sessionClip: sessionClip{ID: "A"}, Rating: 4, VsCurrent: "better"},
			{sessionClip: sessionClip{ID: "B"}, Rating: 2, VsCurrent: "worse"},
		}}}
	pairs := prefPairsFromSessions(ss)
	// A>current, current>B (worse), A>B (ratings differ) = 3 pairs
	if len(pairs) != 3 {
		t.Fatalf("want 3 pairs, got %d: %+v", len(pairs), pairs)
	}
}

func TestPrefPairsFromVerdicts(t *testing.T) {
	vs := []earVerdict{{Family: roster.Flute, Reference: "flute-A4", Options: []instrument.ID{roster.Flute, roster.FluteBreathy}, Verdict: "2", Chosen: roster.FluteBreathy}}
	pairs := prefPairsFromVerdicts(vs)
	if len(pairs) != 1 || pairs[0].Chosen.ID != "flute-breathy" || pairs[0].Rejected.ID != "flute" {
		t.Fatalf("got %+v", pairs)
	}
}
