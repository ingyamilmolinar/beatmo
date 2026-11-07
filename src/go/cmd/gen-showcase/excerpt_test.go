package main

import (
	"testing"
)

// A multi-movement MusicXML file RESTARTS its measure numbering each movement, so "measures 1-10" names several
// passages; these tests relate a circuit's size to the passage it was cut from.

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

// The loop is the excerpt's notated length EXACTLY, re-derived from each bar's meter: rounding up to whole 4/4 bars
// left a 2/4 excerpt out of meter at every repeat, which a groove layer on the melody's downbeats cannot survive.
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
				// A leading pickup wraps to the end, ahead of bar one.
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

// Multi-movement quartets must split; an engraver's repeat-ending number ("X1") must not (an over-eager rule cut Aloha
// Oe to four bars).
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

// A template starts on its first selected bar's downbeat, so the range must start where the music does, not on dead air.
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
