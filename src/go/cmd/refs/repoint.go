//go:build !js

package main

import (
	"fmt"
	"slices"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// repointInstrument makes winnerID the ONLY reference that is ground truth
// for inst, and returns the resulting ref list. It is the shared body of
// both commands that change which clip an instrument is judged against:
// `refs choose` (promote an already-catalogued candidate) and `refs adopt`
// (bring in a local recording).
//
// Every other non-status ref serving inst loses it, and is recorded on the
// winner as an Alternative carrying reason -- so the decision is reversible,
// and so the losers survive as labelled data for the analysis tooling. A ref
// left serving nothing is dropped: it would fail Cat.Validate on the next
// load.
//
// The input slice and every Instruments slice inside it are treated as
// READ-ONLY. Callers reach here holding refs that can still alias the
// package-level memoized wavref.Catalogue() singleton's backing arrays (a
// shallow append([]Ref(nil), c.Refs...) copies the outer slice but not each
// Ref's nested Instruments header), and writing through that would silently
// corrupt every later Catalogue() call in the same process -- invisible in a
// one-shot CLI run, but order-dependent across a `go test` binary. So this
// builds a fresh slice and uses withoutString, which allocates rather than
// filtering in place.
func repointInstrument(refs []wavref.Ref, inst instrument.ID, winnerID, reason string) []wavref.Ref {
	out := make([]wavref.Ref, 0, len(refs))
	var alts []wavref.Alternative

	for _, r := range refs {
		if r.ID != winnerID && r.Status == "" && servesInstrument(r, inst) {
			if r.Source != nil {
				alts = append(alts, wavref.Alternative{URL: r.Source.URL, Reason: reason})
			}
			r.Instruments = slices.DeleteFunc(slices.Clone(r.Instruments), func(id instrument.ID) bool { return id == inst })
			if r.Tier == wavref.TierInstrument && len(r.Instruments) == 0 {
				continue
			}
		}
		out = append(out, r)
	}

	for i := range out {
		if out[i].ID == winnerID {
			// append to a COPY: out[i].Alternatives can still alias
			// the memoized catalogue's backing array.
			merged := make([]wavref.Alternative, 0, len(out[i].Alternatives)+len(alts))
			merged = append(merged, out[i].Alternatives...)
			merged = append(merged, alts...)
			if len(merged) > 0 {
				out[i].Alternatives = merged
			}
		}
	}
	return out
}

// countAlternatives reports how many refs lost inst to winnerID, for the
// human-readable confirmation both callers print.
func countAlternatives(refs []wavref.Ref, inst instrument.ID, winnerID string) int {
	n := 0
	for _, r := range refs {
		if r.ID != winnerID && r.Status == "" && servesInstrument(r, inst) && r.Source != nil {
			n++
		}
	}
	return n
}

// repointReason is the Alternative reason recorded when a catalogued
// candidate wins an ear comparison.
func repointReason(inst instrument.ID, winnerID string) string {
	return fmt.Sprintf("not chosen for %s by ear; %s won", inst, winnerID)
}
