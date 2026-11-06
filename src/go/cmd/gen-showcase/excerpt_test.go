package main

import (
	"testing"
)

// excerpt_test.go — does a template contain the bars the spec asked for, and
// only those?
//
// A published multi-movement work is one MusicXML file whose measure numbering
// RESTARTS each movement, so "measures 1-10" names four different passages in a
// quartet. Selecting on the printed number alone took all of them: the five
// four-movement chamber works in the corpus generated 336-668 bar circuits for
// 8-10 bar excerpts, carrying material from every movement at once. Nothing
// noticed, because no test related the SIZE of a generated circuit to the size
// of the passage it was cut from.

// TestExcerptStaysInOneMovement is the direct property: every note the importer
// keeps must come from the movement the spec selects.
func TestExcerptStaysInOneMovement(t *testing.T) {
	for _, spec := range templateSpecs() {
		spec := spec
		t.Run(spec.Stem, func(t *testing.T) {
			sc := mustScore(t, spec.ScoreID)
			want := specMovement(spec)
			for _, pm := range spec.Parts {
				if pm.Index < 0 || pm.Index >= len(sc.Parts) {
					continue
				}
				for _, n := range sc.Parts[pm.Index].Notes {
					if !inRange(n, spec) {
						continue
					}
					if n.Movement != want {
						t.Fatalf("kept a note from movement %d (measure %d) while selecting movement %d",
							n.Movement, n.Measure, want)
					}
				}
			}
		})
	}
}

// TestLoopLengthMatchesExcerpt relates the generated circuit's LENGTH to the
// notated length of the passage. It is the cheap invariant that would have
// caught the movement leak on the day it appeared: a 10-bar excerpt cannot
// honestly produce a 370-bar loop.
//
// The loop is the excerpt's notated length EXACTLY. It used to be rounded up
// to whole 4/4 bars from the last sounding note, so a 15-bar 2/4 excerpt
// looped with a silent half bar and fell out of meter at every repeat — and a
// groove layer, which must land on the melody's downbeats, cannot survive that.
// The expectation is re-derived from each bar's METER here (the generator sums
// measured bar lengths), so the two agree only when the excerpt is made of
// whole bars of the meter in force.
func TestLoopLengthMatchesExcerpt(t *testing.T) {
	for _, spec := range templateSpecs() {
		spec := spec
		t.Run(spec.Stem, func(t *testing.T) {
			sh, _, err := buildFromScore(spec)
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			sc := mustScore(t, spec.ScoreID)
			part := sc.Parts[spec.Parts[0].Index]
			quarters, bars := 0.0, 0
			for _, m := range part.Measures {
				if m.Movement != specMovement(spec) || m.Number < spec.From || m.Number > spec.To {
					continue
				}
				bars++
				// A leading pickup is not part of the loop's length: it wraps
				// to the end, ahead of bar one.
				if bars == 1 && spec.To > spec.From && m.LengthDiv < part.Divisions*4*m.Beats/m.BeatType {
					continue
				}
				quarters += 4 * float64(m.Beats) / float64(m.BeatType)
			}
			if bars != spec.To-spec.From+1 {
				t.Fatalf("score has %d bars numbered %d..%d in movement %d, want %d",
					bars, spec.From, spec.To, specMovement(spec), spec.To-spec.From+1)
			}
			want := int(quarters * float64(spec.Subdiv/4))
			if sh.TotalSteps != want {
				t.Errorf("%d-bar excerpt (%.2f quarters) produced a %d-step loop, want %d",
					bars, quarters, sh.TotalSteps, want)
			}
		})
	}
}

// TestMovementBoundariesAreRealRestarts pins the detection rule itself against
// the live corpus: the multi-movement quartets must split, and a score whose
// numbering merely carries an engraver's repeat-ending exception ("X1") must
// not. Aloha Oe is the second case, and an over-eager rule cut it to its first
// four bars.
func TestMovementBoundariesAreRealRestarts(t *testing.T) {
	for _, c := range []struct {
		score string
		multi bool
	}{
		{"mozart-quartet-k387", true},
		{"borodin-quartet-2", true},
		{"dvorak-american", true},
		{"liliuokalani-aloha-oe", false},
		{"bach-brandenburg-3", false},
		{"schubert-erlkonig", false},
	} {
		sc := mustScore(t, c.score)
		seen := map[int]bool{}
		for _, p := range sc.Parts {
			for _, n := range p.Notes {
				seen[n.Movement] = true
			}
		}
		if got := len(seen) > 1; got != c.multi {
			t.Errorf("%s: movements=%d, want %s", c.score, len(seen),
				map[bool]string{true: "more than one", false: "exactly one"}[c.multi])
		}
	}
}

// TestExcerptOpensWithSound: a template starts on its first selected bar's
// downbeat, so a spec whose first bar is silent in every mapped part ships a
// circuit that opens with dead air. The loop used to hide that by anchoring on
// the first sounding note — which also slid every bar line off the score's.
// The range itself must start where the music does.
func TestExcerptOpensWithSound(t *testing.T) {
	for _, spec := range templateSpecs() {
		sc := mustScore(t, spec.ScoreID)
		bars := map[int]bool{}
		for _, pm := range spec.Parts {
			for _, n := range sc.Parts[pm.Index].Notes {
				if !n.Rest && inRange(n, spec) {
					bars[n.Measure] = true
				}
			}
		}
		if !bars[spec.From] {
			opens := spec.To + 1
			for m := spec.From; m <= spec.To; m++ {
				if bars[m] {
					opens = m
					break
				}
			}
			t.Errorf("%s: bar %d is silent in every mapped part; the music starts at bar %d", spec.Stem, spec.From, opens)
		}
	}
}
