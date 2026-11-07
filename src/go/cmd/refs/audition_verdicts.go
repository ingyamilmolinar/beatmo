//go:build !js

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// The labelled corpus the analysis-metric validation reads.
const verdictsRel = "internal/audio/testdata/ear_verdicts.jsonl"

// earVerdict is one line of the verdicts JSONL file: one listener's
// judgement of one family's synth option(s) against its real reference.
type earVerdict struct {
	Family      instrument.ID   `json:"family"`
	Reference   string          `json:"reference"`
	Options     []instrument.ID `json:"options"`
	Verdict     string          `json:"verdict"`
	Chosen      instrument.ID   `json:"chosen,omitempty"`
	Note        string          `json:"note,omitempty"`
	Tags        []string        `json:"tags,omitempty"`
	SemisFromA3 int             `json:"semis_from_a3"`
	// The chosen member's sound hash (InstrumentSoundHash), so refs validate can tell a verdict on an old document from a
	// current one. May be empty on old rows.
	RecipeSHA string `json:"recipe_sha,omitempty"`
}

func marshalVerdict(v earVerdict) ([]byte, error) {
	return json.Marshal(v)
}

func unmarshalVerdict(b []byte) (earVerdict, error) {
	var v earVerdict
	if err := json.Unmarshal(b, &v); err != nil {
		return earVerdict{}, err
	}
	return v, nil
}

// From the module root, so it works from src/go (go run) and from cmd/refs (go test).
func defaultVerdictsPath() (string, error) {
	root, ok := wavref.ModuleRoot()
	if !ok {
		return "", fmt.Errorf("refs audition: cannot locate the module root (no go.mod above cwd)")
	}
	return filepath.Join(root, verdictsRel), nil
}

// Appended at once: an interrupted session loses nothing already recorded.
func appendVerdict(path string, v earVerdict) error {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("refs audition: mkdir %s: %w", dir, err)
		}
	}
	b, err := marshalVerdict(v)
	if err != nil {
		return fmt.Errorf("refs audition: marshal verdict: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("refs audition: open %s: %w", path, err)
	}
	defer f.Close()
	if _, err := f.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("refs audition: write %s: %w", path, err)
	}
	return nil
}

// recordVerdict SUPERSEDES the family's existing row: re-auditioning after a render change is the workflow, and two rows
// in the labelled ground truth would differ only by line order. A note survives a re-recording with the SAME verdict and
// is dropped when it changes (prose about "too buzzy" must not reattach to a "yes"); a new note always wins.
func recordVerdict(path string, v earVerdict) error {
	existing, err := readVerdicts(path)
	if err != nil {
		return err
	}
	for i := range existing {
		if existing[i].Family != v.Family {
			continue
		}
		if v.Note == "" && existing[i].Verdict == v.Verdict {
			v.Note, v.Tags = existing[i].Note, existing[i].Tags
		}
		existing[i] = v
		return writeVerdictsAtomic(path, existing)
	}
	return appendVerdict(path, v)
}

// A missing file is zero verdicts: a fresh session has not created it yet.
func readVerdicts(path string) ([]earVerdict, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("refs audition: open %s: %w", path, err)
	}
	defer f.Close()

	var out []earVerdict
	sc := bufio.NewScanner(f)
	// Notes are free text and can run long.
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		v, err := unmarshalVerdict(line)
		if err != nil {
			return nil, fmt.Errorf("refs audition: parse %s: %w", path, err)
		}
		out = append(out, v)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("refs audition: read %s: %w", path, err)
	}
	return out, nil
}

// loadAnsweredFamilies returns the set of family keys with at least one
// recorded verdict in path.
func loadAnsweredFamilies(path string) (map[instrument.ID]bool, error) {
	verdicts, err := readVerdicts(path)
	if err != nil {
		return nil, err
	}
	out := make(map[instrument.ID]bool, len(verdicts))
	for _, v := range verdicts {
		out[v.Family] = true
	}
	return out, nil
}

// An explicit instrument always wins over resume and the session skip. Otherwise the first family in Key order that is
// neither answered (with resume, persisted) nor in sessionSkip (ab-all's in-session "s" list, never persisted, so a
// later run offers it again). found=false with nil error = nothing left to do.
func pickFamily(families []auditionFamily, inst instrument.ID, resume bool, answered, sessionSkip map[instrument.ID]bool) (auditionFamily, bool, error) {
	if inst != "" {
		for _, f := range families {
			if f.Key == inst {
				return f, true, nil
			}
		}
		return auditionFamily{}, false, fmt.Errorf("refs audition: %q is not a known family (see `refs audition -list`)", inst)
	}
	for _, f := range families {
		if resume && answered[f.Key] {
			continue
		}
		if sessionSkip[f.Key] {
			continue
		}
		return f, true, nil
	}
	return auditionFamily{}, false, nil
}

// Empty entries are ignored, so the Makefile's accumulator can grow by plain concatenation.
func parseSkipSet(raw string) map[instrument.ID]bool {
	out := map[instrument.ID]bool{}
	for _, key := range strings.Split(raw, ",") {
		key = strings.TrimSpace(key)
		if key != "" {
			out[instrument.ID(key)] = true
		}
	}
	return out
}
