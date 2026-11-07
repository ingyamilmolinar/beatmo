//go:build !js

package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// The golden makes a new instrument without a reference decision fail the build; coverage is per ROSTER id, never folded.
func TestCoverage_ClassifiesEveryBaseInstrument(t *testing.T) {
	rows, err := coverage()
	if err != nil {
		t.Fatalf("coverage: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("coverage returned no rows")
	}
	seen := map[string]bool{}
	for _, r := range rows {
		seen[string(r.Instrument)] = true
		switch r.State {
		case "covered", "synthetic", "needs-source", "unclassified":
		default:
			t.Errorf("%s: unexpected state %q", r.Instrument, r.State)
		}
	}
	for _, id := range roster.All {
		if !seen[string(id)] {
			t.Errorf("instrument %q missing from coverage report", id)
		}
	}
}

// Folding an id onto a shorter one hid every variant behind its base, so one with no reference looked covered.
func TestCoverageReportsRenamedVariantsSeparately(t *testing.T) {
	rows, err := coverage()
	if err != nil {
		t.Fatalf("coverage: %v", err)
	}
	seen := map[string]bool{}
	for _, r := range rows {
		seen[string(r.Instrument)] = true
	}
	for _, id := range []instrument.ID{roster.Snare, roster.SnareLofi, roster.SnareCrack} {
		if !seen[string(id)] {
			t.Errorf("%s missing from the coverage report", id)
		}
	}
}

func TestRunDoctor_MatchesGolden(t *testing.T) {
	var buf bytes.Buffer
	if err := runDoctor(&buf); err != nil {
		t.Fatalf("runDoctor: %v", err)
	}
	want, err := os.ReadFile("testdata/doctor_golden.txt")
	if err != nil {
		t.Fatalf("read golden: %v (regenerate with: go run ./cmd/refs doctor > cmd/refs/testdata/doctor_golden.txt)", err)
	}
	if strings.TrimSpace(buf.String()) != strings.TrimSpace(string(want)) {
		t.Errorf("doctor output drifted from golden.\n--- got ---\n%s\n--- want ---\n%s", buf.String(), want)
	}
}

// The catalogue must make a decision about every instrument. An
// unclassified instrument means someone added an instrument without
// deciding what its ground truth is.
func TestCoverage_NoUnclassifiedInstruments(t *testing.T) {
	rows, err := coverage()
	if err != nil {
		t.Fatalf("coverage: %v", err)
	}
	var bad []string
	for _, r := range rows {
		if r.State == "unclassified" {
			bad = append(bad, string(r.Instrument))
		}
	}
	if len(bad) > 0 {
		t.Errorf("%d instrument(s) unclassified in catalogue.json: %v", len(bad), bad)
	}
}

// A needs-source or synthetic entry without a reason is an unexplained
// gap; the note is what makes the classification reviewable.
func TestCoverage_StatusRowsExplainThemselves(t *testing.T) {
	rows, err := coverage()
	if err != nil {
		t.Fatalf("coverage: %v", err)
	}
	for _, r := range rows {
		if (r.State == "synthetic" || r.State == "needs-source") && r.Note == "" {
			t.Errorf("%s is %s but carries no note explaining why", r.Instrument, r.State)
		}
	}
}

// The reverse error coverage() cannot see: a catalogue entry naming an id not in the roster, which nothing can resolve.
func TestCatalogue_EveryReferencedInstrumentExistsInRoster(t *testing.T) {
	c, err := loadCatalogue()
	if err != nil {
		t.Fatalf("loadCatalogue: %v", err)
	}
	known := map[string]bool{}
	for _, id := range roster.All {
		known[string(id)] = true
	}
	for _, r := range c.Refs {
		for _, inst := range r.Instruments {
			if !known[string(inst)] {
				t.Errorf("catalogue entry %q references instrument %q, which does not exist in roster.All", r.ID, inst)
			}
		}
	}
}
