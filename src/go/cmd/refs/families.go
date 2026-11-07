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

// Roster instruments modelling the SAME real instrument (cello / cello-warm) are compared together against one shared
// reference, rather than judging one real instrument twice against two recordings.
type auditionFamily struct {
	// The shortest member id (ties alphabetical), which is the base instrument in every current family.
	Key instrument.ID
	// Members lists every roster instrument in the family, sorted
	// alphabetically. A family with exactly one member has no sibling.
	Members []instrument.ID
	// Every member renders against it at its own Measured pitch, so the comparison is like for like.
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

// The SINGLE source of truth for which instruments belong together, over comparableInstruments: a ref whose Instruments
// list names 2+ covered ids (oboe-A4 → oboe-full, oboe), and "X-<qualifier>" joining a covered base "X" (cello-warm).
// Sorted by Key.
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

	// Only the LAST hyphen segment, and only onto a covered base, which keeps dnb-kick out of a nonexistent "kick" family.
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

// A name matching no family is an ERROR: a typo would cost a long listening session on the wrong instruments.
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

// The shortest member id, ties alphabetical: the natural base instrument in every current family.
func familyKey(members []instrument.ID) instrument.ID {
	key := members[0]
	for _, m := range members[1:] {
		if len(m) < len(key) || (len(m) == len(key) && m < key) {
			key = m
		}
	}
	return key
}

// The ref covering the MOST members, ties by smallest id, so the choice never depends on catalogue order.
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
