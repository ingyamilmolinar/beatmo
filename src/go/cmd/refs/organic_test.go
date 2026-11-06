//go:build !js

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

func TestFormatOrganicTable_PlacesEveryOrganicAxis(t *testing.T) {
	r := fingerprint.OrganicRange{}
	for _, id := range fingerprint.OrganicIDs() {
		r[id] = fingerprint.RangeEntry{P10: 0, P50: 1, P90: 2, N: fingerprint.OrganicMinN}
	}
	vals := map[string]organicValue{}
	for _, id := range fingerprint.OrganicIDs() {
		vals[id] = organicValue{V: 3, OK: true}
	}
	var buf bytes.Buffer
	formatOrganicTable(&buf, "test-inst", vals, r)
	out := buf.String()
	if strings.Count(out, "above") != len(fingerprint.OrganicIDs()) {
		t.Fatalf("every axis should read 'above':\n%s", out)
	}
}

// A locally adopted ref (`refs adopt`, Source.Provider == "local") is
// catalogued in the committed catalogue.json but its audio lives at a path on
// one machine, so `refs fetch` cannot materialize it anywhere else. It must
// therefore stay OUT of the committed organic_range.json baseline that
// `refs check` re-derives (locally via `make refs-check`, and in CI after the
// corpus is fetched). Adopting one snare on 2026-09-14 moved 10 descriptors'
// percentiles and every one of their n by +1.
func TestLocallyAdoptedRefsAreExcludedFromTheCommittedBaseline(t *testing.T) {
	cases := []struct {
		name string
		ref  wavref.Ref
		want bool
	}{
		{"fetched from a public provider", wavref.Ref{ID: "snare-vcsl", Source: &wavref.Source{Provider: "vcsl"}}, false},
		{"adopted from a local path", wavref.Ref{ID: "snare-acoustic-local", Source: &wavref.Source{Provider: "local"}}, true},
		{"no source block at all (a status-only entry)", wavref.Ref{ID: "world-folk-needs-source"}, false},
		{"empty provider", wavref.Ref{ID: "x", Source: &wavref.Source{}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := locallyAdopted(tc.ref); got != tc.want {
				t.Errorf("locallyAdopted(%s) = %v, want %v", tc.ref.ID, got, tc.want)
			}
		})
	}
}
