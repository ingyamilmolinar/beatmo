//go:build !js

package main

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// auditionFamily groups roster instruments that model the SAME real
// instrument -- e.g. cello and cello-warm, or oboe and oboe-full -- so
// `refs audition` compares them together against one shared reference
// instead of asking the listener to judge the same real instrument twice
// against two different recordings.
type auditionFamily struct {
	// Key is the family's display name: the shortest member id (ties
	// broken alphabetically), which in every family in the current
	// catalogue happens to be the "base" instrument (cello, oboe, conga,
	// tom, ...).
	Key instrument.ID
	// Members lists every roster instrument in the family, sorted
	// alphabetically. A family with exactly one member has no sibling.
	Members []instrument.ID
	// Reference is the single wavref.Ref every member is rendered against,
	// at its own Measured pitch, so the family's comparison is
	// like-for-like.
	Reference wavref.Ref
}

// familyUnionFind is a tiny union-find over instrument ids, scoped to one
// buildAuditionFamilies call -- not worth a general-purpose package for a
// few dozen elements.
type familyUnionFind struct {
	parent map[instrument.ID]instrument.ID
}

func newFamilyUnionFind() *familyUnionFind {
	return &familyUnionFind{parent: map[instrument.ID]instrument.ID{}}
}

func (u *familyUnionFind) find(x instrument.ID) instrument.ID {
	if _, ok := u.parent[x]; !ok {
		u.parent[x] = x
	}
	if u.parent[x] != x {
		u.parent[x] = u.find(u.parent[x])
	}
	return u.parent[x]
}

func (u *familyUnionFind) union(a, b instrument.ID) {
	ra, rb := u.find(a), u.find(b)
	if ra != rb {
		u.parent[ra] = rb
	}
}

// buildAuditionFamilies is the SINGLE source of truth for "which roster
// instruments belong together". It starts from comparableInstruments (every
// covered, non-status instrument) and unions two ways:
//
//  1. Instruments-array route: any non-status instrument-tier ref whose
//     Instruments array lists 2+ covered instruments -- they're already
//     grouped by the catalogue itself (e.g. oboe-A4 -> [oboe-full, oboe],
//     conga-freesound-alt -> [conga, conga-open]).
//  2. Naming-convention route: instrument "X-<qualifier>" joins base "X"
//     when both are covered (e.g. cello + cello-warm, conga-tumba joining
//     the conga family via its "conga" prefix even though its own
//     reference is different from conga-freesound-alt's).
//
// Families are returned sorted by Key.
func buildAuditionFamilies(c wavref.Cat) ([]auditionFamily, error) {
	covered := comparableInstruments(c)
	coveredSet := make(map[instrument.ID]bool, len(covered))
	for _, inst := range covered {
		coveredSet[inst] = true
	}

	uf := newFamilyUnionFind()
	for _, inst := range covered {
		uf.find(inst)
	}

	// Route 1: instruments-array grouping.
	for _, r := range c.Refs {
		if r.Status != "" {
			continue
		}
		var first instrument.ID
		for _, inst := range r.Instruments {
			if !coveredSet[inst] {
				continue
			}
			if first == "" {
				first = inst
				continue
			}
			uf.union(first, inst)
		}
	}

	// Route 2: naming-convention grouping. Only the LAST hyphen segment is
	// stripped, and only when the resulting base is itself covered -- this
	// is what keeps dnb-kick/zgump-kick from being folded into a
	// nonexistent "kick" family.
	for _, inst := range covered {
		idx := strings.LastIndex(string(inst), "-")
		if idx < 0 {
			continue
		}
		base := inst[:idx]
		if base != inst && coveredSet[base] {
			uf.union(inst, base)
		}
	}

	groups := map[instrument.ID][]instrument.ID{}
	for _, inst := range covered {
		root := uf.find(inst)
		groups[root] = append(groups[root], inst)
	}

	families := make([]auditionFamily, 0, len(groups))
	for _, members := range groups {
		slices.Sort(members)
		ref, err := pickFamilyReference(c, members)
		if err != nil {
			return nil, err
		}
		families = append(families, auditionFamily{
			Key:       familyKey(members),
			Members:   members,
			Reference: ref,
		})
	}
	sort.Slice(families, func(i, j int) bool { return families[i].Key < families[j].Key })
	return families, nil
}

// filterFamilies restricts families to the comma-separated set in raw, leaving
// them in their existing (Key-sorted) order. An empty raw is a no-op.
//
// Used by `-families` to re-audition a targeted subset. An entry naming no
// known family is an ERROR rather than a silent no-match: a typo would
// otherwise cost a long listening session spent on the wrong instruments, or
// none at all.
func filterFamilies(families []auditionFamily, raw string) ([]auditionFamily, error) {
	want := parseSkipSet(raw) // same "comma-separated, tolerate empties" parsing
	if len(want) == 0 {
		return families, nil
	}
	known := make(map[instrument.ID]bool, len(families))
	for _, f := range families {
		known[f.Key] = true
	}
	var unknown []string
	for k := range want {
		if !known[k] {
			unknown = append(unknown, string(k))
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return nil, fmt.Errorf("refs audition: -families names %d unknown famil(y/ies): %s "+
			"(run `refs audition -sh` or see `refs compare -list` for valid keys)",
			len(unknown), strings.Join(unknown, ", "))
	}
	out := make([]auditionFamily, 0, len(want))
	for _, f := range families {
		if want[f.Key] {
			out = append(out, f)
		}
	}
	return out, nil
}

// familyKey picks the family's display name: the shortest member id, ties
// broken alphabetically. In every family the current catalogue produces,
// this is the natural "base" instrument (cello over cello-warm, tom over
// high-tom-organic, conga over conga-open/conga-tumba, ...).
func familyKey(members []instrument.ID) instrument.ID {
	key := members[0]
	for _, m := range members[1:] {
		if len(m) < len(key) || (len(m) == len(key) && m < key) {
			key = m
		}
	}
	return key
}

// pickFamilyReference chooses ONE reference for every member of a family to
// render against. Candidates are every non-status ref that covers at least
// one member; the winner is the one covering the MOST members (the
// reference "the most members share"), tie-broken by the smallest ref id so
// the choice is deterministic and doesn't depend on catalogue.json's
// ordering.
func pickFamilyReference(c wavref.Cat, members []instrument.ID) (wavref.Ref, error) {
	memberSet := make(map[instrument.ID]bool, len(members))
	for _, m := range members {
		memberSet[m] = true
	}

	candidates := make([]wavref.Ref, 0, len(c.Refs))
	for _, r := range c.Refs {
		if r.Status != "" {
			continue
		}
		for _, inst := range r.Instruments {
			if memberSet[inst] {
				candidates = append(candidates, r)
				break
			}
		}
	}
	if len(candidates) == 0 {
		return wavref.Ref{}, fmt.Errorf("refs audition: no reference covers any of %v", members)
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })

	shareCount := func(r wavref.Ref) int {
		n := 0
		for _, inst := range r.Instruments {
			if memberSet[inst] {
				n++
			}
		}
		return n
	}

	best := candidates[0]
	bestShare := shareCount(best)
	for _, r := range candidates[1:] {
		if s := shareCount(r); s > bestShare {
			best, bestShare = r, s
		}
	}
	return best, nil
}
