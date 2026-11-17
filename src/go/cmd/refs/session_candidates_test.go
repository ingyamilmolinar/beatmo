//go:build !js

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCandidates_ValidatesShape(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.json")
	os.WriteFile(p, []byte(`{"x":{"reference":"r","base":"b","candidates":[{"id":"A","label":"l","hypothesis":"h","overrides":{"k":1}},{"id":"A","label":"l","hypothesis":"h","overrides":{}}]}}`), 0o644)
	if _, err := loadCandidates(p); err == nil {
		t.Fatal("duplicate candidate id must be rejected")
	}
	os.WriteFile(p, []byte(`{"x":{"reference":"r","base":"b","candidates":[{"id":"A","label":"l","hypothesis":"h","overrides":{"k":1}}]}}`), 0o644)
	got, err := loadCandidates(p)
	if err != nil || got["x"].Candidates[0].Overrides["k"] != 1 {
		t.Fatalf("valid file failed: %v %+v", err, got)
	}
}

func TestShippedCandidatesFile_NoPlaceholders(t *testing.T) {
	p, err := defaultCandidatesPath()
	if err != nil {
		t.Fatal(err)
	}
	fams, err := loadCandidates(p)
	if err != nil {
		t.Fatal(err)
	}
	for name, f := range fams {
		if len(f.Candidates) != 2 {
			t.Errorf("%s: spec requires exactly two candidates, got %d", name, len(f.Candidates))
		}
		for _, c := range f.Candidates {
			if c.Label == "TBD-by-task-15" || c.Hypothesis == "TBD-by-task-15" {
				t.Errorf("%s/%s still carries the Task 13 placeholder; Task 15 must replace it", name, c.ID)
			}
		}
	}
}
