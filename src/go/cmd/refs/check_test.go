//go:build !js

package main

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// The checks themselves need the reference corpus, so no test runs them for
// real (tests never read reference audio). What is tested is everything around
// them: an absent corpus SKIPS loudly instead of passing, and the comparisons
// catch every kind of drift.

func TestRefsCheckSkipsEveryCheckWithoutACorpus(t *testing.T) {
	t.Setenv(wavref.EnvVar, t.TempDir())
	var out strings.Builder
	if err := runChecks(&out, nil, false); err != nil {
		t.Fatalf("runChecks with no corpus: %v\n%s", err, out.String())
	}
	for _, c := range corpusChecks {
		if !strings.Contains(out.String(), "SKIP  "+c.name) {
			t.Errorf("check %q did not report SKIP with no corpus:\n%s", c.name, out.String())
		}
	}
	if strings.Contains(out.String(), "ok    ") {
		t.Errorf("a check reported ok with no corpus -- a silent pass:\n%s", out.String())
	}
}

func TestRefsCheckRejectsAnUnknownCheckName(t *testing.T) {
	if err := cmdCheck([]string{"-only", "thresholds,nope"}); err == nil || !strings.Contains(err.Error(), `"nope"`) {
		t.Fatalf("cmdCheck(-only nope) = %v, want an unknown-check error", err)
	}
}

func TestDiffThresholdFilesReportsEveryKindOfDrift(t *testing.T) {
	base := fingerprint.ThresholdEntry{N: 5, Source: "corpus", Drift: 0.1, Mismatch: 0.3}
	want := fingerprint.ThresholdFile{"same": base, "n": base, "src": base, "val": base, "gone": base}
	got := fingerprint.ThresholdFile{"same": base, "new": base}
	g := base
	g.N = 6
	got["n"] = g
	g = base
	g.Source = "probe"
	got["src"] = g
	g = base
	g.Mismatch = 0.31
	got["val"] = g

	mis := diffThresholdFiles(got, want)
	joined := strings.Join(mis, "|")
	for _, id := range []string{"n (", "src (", "val (", "gone (", "new ("} {
		if !strings.Contains(joined, id) {
			t.Errorf("drift %q not reported: %v", id, mis)
		}
	}
	if strings.Contains(joined, "same (") {
		t.Errorf("an identical entry was reported as drift: %v", mis)
	}
	if len(diffThresholdFiles(want, want)) != 0 {
		t.Error("a file differs from itself")
	}
}

func TestDiffOrganicRangesReportsDriftAndStaleIDs(t *testing.T) {
	ids := fingerprint.OrganicIDs()
	if len(ids) < 2 {
		t.Fatalf("need >= 2 organic ids, have %d", len(ids))
	}
	e := fingerprint.RangeEntry{N: 4, P10: 1, P50: 2, P90: 3}
	want := fingerprint.OrganicRange{ids[0]: e, ids[1]: e, "stale-only-committed": e}
	moved := e
	moved.P50 = 2.5
	got := fingerprint.OrganicRange{ids[0]: e, ids[1]: moved, "stale-only-fresh": e}

	mis := diffOrganicRanges(got, want)
	for _, id := range []string{ids[1], "stale-only-committed", "stale-only-fresh"} {
		found := false
		for _, m := range mis {
			found = found || m == id
		}
		if !found {
			t.Errorf("%q not reported: %v", id, mis)
		}
	}
	for _, m := range mis {
		if m == ids[0] {
			t.Errorf("an identical range %q was reported as drift: %v", ids[0], mis)
		}
	}
	clean := fingerprint.OrganicRange{ids[0]: e, ids[1]: e}
	if mis := diffOrganicRanges(clean, clean); len(mis) != 0 {
		t.Errorf("a range file differs from itself: %v", mis)
	}
	// An id OrganicIDs() no longer covers is stale even when both files carry
	// it: the committed golden must be regenerated without it.
	if mis := diffOrganicRanges(want, want); len(mis) != 1 || mis[0] != "stale-only-committed" {
		t.Errorf("stale id in both files: got %v, want [stale-only-committed]", mis)
	}
}

// TestF0CheckTablesNameCataloguedRefs: the f0 checks skip a clip that is not
// on this machine, so an id the catalogue no longer has is skipped forever
// with no message -- ride-vcsl stayed in unpitchedRefs after ride-local
// replaced it on 2026-09-22, and the separation check quietly lost its only
// real ride. Reads the committed catalogue only, never audio.
func TestF0CheckTablesNameCataloguedRefs(t *testing.T) {
	c, err := wavref.Catalogue()
	if err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{}
	for _, r := range c.Refs {
		known[r.ID] = true
	}
	ids := append(append([]string(nil), unpitchedRefs...), pitchedRefs...)
	for id := range lowConfidencePitchedRefs {
		ids = append(ids, id)
	}
	for _, id := range ids {
		if !known[id] {
			t.Errorf("f0 check names %q, which is not in internal/wavref/catalogue.json", id)
		}
	}
}
