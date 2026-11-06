//go:build !js

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// candidatesRel is the module-root-relative path to the shipped audition
// candidates file: one entry per family under audition, each naming the
// reference ref, the base instrument to render, and a small set of
// override candidates to A/B against that base.
const candidatesRel = "internal/audio/testdata/audition_candidates.json"

// candidateSpec is one named parameter-override variant to audition against
// a family's base render.
type candidateSpec struct {
	ID         string             `json:"id"`
	Label      string             `json:"label"`
	Hypothesis string             `json:"hypothesis"`
	Overrides  map[string]float64 `json:"overrides"`

	// A WAV-clone candidate (`refs session -clone`, never in the file): a
	// whole document rendered at its own fit pitch, identified by DocSHA.
	Doc      *instrument.Doc `json:"-"`
	DocSHA   string          `json:"-"`
	FitPitch float64         `json:"-"`
	// SamePitch: render the document at the current voice's note (a tuned
	// candidate keeps the shipped pitch frame) instead of FitPitch.
	SamePitch bool `json:"-"`
}

// familyCandidates is the audition set for one instrument family: the
// reference wavref ID to compare against, the base instrument whose recipe
// the candidates override, and the candidates themselves.
type familyCandidates struct {
	Reference  string          `json:"reference"`
	Base       instrument.ID   `json:"base"`
	Candidates []candidateSpec `json:"candidates"`
}

// defaultCandidatesPath resolves the shipped candidates file relative to the
// module root, mirroring defaultOrganicRangePath in organic.go.
func defaultCandidatesPath() (string, error) {
	root, ok := wavref.ModuleRoot()
	if !ok {
		return "", fmt.Errorf("refs session: cannot locate the module root (no go.mod above cwd)")
	}
	return filepath.Join(root, candidatesRel), nil
}

// loadCandidates reads and validates a candidates file: every family needs a
// non-empty Reference and Base, at least one candidate, and every candidate
// needs a unique ID plus non-empty Label and Hypothesis. A candidate with no
// overrides would render identically to the base, so that's rejected too.
func loadCandidates(path string) (map[instrument.ID]familyCandidates, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out map[instrument.ID]familyCandidates
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("refs session: %s: %w", path, err)
	}
	for name, f := range out {
		if f.Reference == "" || f.Base == "" {
			return nil, fmt.Errorf("refs session: %s: reference and base are required", name)
		}
		if len(f.Candidates) == 0 {
			return nil, fmt.Errorf("refs session: %s: at least one candidate is required", name)
		}
		seen := map[string]bool{}
		for _, c := range f.Candidates {
			if c.ID == "" || c.Label == "" || c.Hypothesis == "" {
				return nil, fmt.Errorf("refs session: %s: every candidate needs id, label and hypothesis", name)
			}
			if seen[c.ID] {
				return nil, fmt.Errorf("refs session: %s: duplicate candidate id %q", name, c.ID)
			}
			seen[c.ID] = true
			if len(c.Overrides) == 0 {
				return nil, fmt.Errorf("refs session: %s/%s: overrides are empty (would equal the base)", name, c.ID)
			}
		}
	}
	return out, nil
}
