package main

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// TestSwingDelaysExactlyTheSwungPositions checks the shipped swing templates
// against the arithmetic of swing, re-derived here from the ratio and unit
// alone: a pair of swung units (2u steps) splits at 2u·r/(r+1), the first
// half stretching to that split and the second compressing into the rest, so
// a hit at position x in its pair plays warp(x)-x steps late. Every drum-row
// hit must carry exactly that delay — on-beat hits none. A single wrong node
// (swing applied to the downbeat, or leaking to the next node) fails.
func TestSwingDelaysExactlyTheSwungPositions(t *testing.T) {
	type sw struct{ ratio, unitSteps float64 }
	cases := map[string]sw{ // template → its groove's swing, unit in 16th steps
		"funky-drummer":          {13.0 / 11, 1},
		"superstition-groove":    {5.0 / 3, 1},
		"rosanna-shuffle":        {2, 1},
		"jazz-swing":             {2, 2},
		"hook-and-sling":         {13.0 / 11, 1},
		"look-ka-py-py":          {13.0 / 11, 1},
		"synthetic-substitution": {13.0 / 11, 1},
		"expensive-shit":         {13.0 / 11, 1},
		"rock-lobster":           {2.0 / 1, 1},
		"peek-a-boo":             {13.0 / 11, 1},
		"achtung":                {13.0 / 11, 1},
		"raised-by-snakes":       {13.0 / 11, 1},
		"problemes-damour":       {7.0 / 5, 1},
		"clear":                  {7.0 / 5, 1},
		"on-and-on":              {13.0 / 11, 1},
		"can-you-feel-it":        {13.0 / 11, 1},
		"voodoo-ray":             {13.0 / 11, 1},
	}
	for _, spec := range templateSpecs() {
		c, ok := cases[spec.Stem]
		if !ok {
			continue
		}
		delete(cases, spec.Stem)
		sh, _, err := buildFromScore(spec)
		if err != nil {
			t.Fatal(err)
		}
		pair := 2 * c.unitSteps
		split := pair * c.ratio / (c.ratio + 1)
		checked := 0
		for _, r := range sh.Rows {
			if !r.Kit {
				continue
			}
			for _, h := range r.Hits {
				checked++
				x := math.Mod(float64(h.Step), pair)
				warped := split + (x-c.unitSteps)*(pair-split)/c.unitSteps
				if x <= c.unitSteps {
					warped = x * split / c.unitSteps
				}
				want := roundPct(warped - x)
				if want == 0 {
					if h.Groove != "" {
						t.Errorf("%s %s step %d: on-beat hit grooved %s %v", spec.Stem, r.Inst, h.Step, h.Groove, h.GroovePct)
					}
					continue
				}
				if h.Groove != grooveDelay || math.Abs(h.GroovePct-want) > 0.0011 {
					t.Errorf("%s %s step %d: %s %v, want delay %v", spec.Stem, r.Inst, h.Step, h.Groove, h.GroovePct, want)
				}
			}
		}
		if checked == 0 {
			t.Errorf("%s: no drum hits checked", spec.Stem)
		}
	}
	for stem := range cases {
		t.Errorf("swing template %q is not in the spec table", stem)
	}
}

// TestTrapRollPlaysAsTriplets: the trap groove's sixteenth-triplet hi-hat
// roll sits between 32nd grid lines, so its inner strokes must reach the
// circuit as rush/delay of exactly a third of a step — not collapse onto the
// grid, and not be dropped.
func TestTrapRollPlaysAsTriplets(t *testing.T) {
	for _, spec := range templateSpecs() {
		if spec.Stem != "trap-808" {
			continue
		}
		sh, stats, err := buildFromScore(spec)
		if err != nil {
			t.Fatal(err)
		}
		thirds := 0
		for _, r := range sh.Rows {
			if r.Inst != roster.Hihat {
				continue
			}
			for _, h := range r.Hits {
				if h.Groove != "" && math.Abs(h.GroovePct-0.333) < 0.0011 {
					thirds++
				}
			}
		}
		// Four off-grid strokes per roll, one roll per two-bar groove loop.
		if loops := sh.TotalSteps / 64; thirds != 4*loops {
			t.Errorf("%d hi-hat hits a third of a step off the grid, want %d (4 per roll × %d loops)", thirds, 4*loops, loops)
		}
		if stats.GrooveClamped != 0 {
			t.Errorf("clamped %d", stats.GrooveClamped)
		}
		return
	}
	t.Fatal("stars-and-stripes-trap not found")
}
