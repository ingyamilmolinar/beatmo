//go:build !js

package main

import (
	"fmt"
	"slices"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// repointInstrument makes winnerID the ONLY ground truth for inst (refs choose and refs adopt). The losers are recorded
// as Alternatives (reversible, and labelled data); a ref left serving nothing is dropped. Read-only inputs: they can
// alias the memoized catalogue's nested slices, and writing through them makes tests order-dependent.
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
