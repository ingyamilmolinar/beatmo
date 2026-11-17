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

// verdictsRel is the corpus of `refs audition` verdicts, relative to the Go
// module root -- the labelled corpus later analysis-metric validation work
// reads from.
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
	// RecipeSHA is the hash of the exact instrument recipe params the chosen
	// member was heard against (InstrumentSoundHash), so `refs validate` can
	// tell a verdict recorded against an old seed apart from one still
	// current. Empty for verdicts recorded before this field existed.
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

// defaultVerdictsPath resolves the committed verdicts file location from
// the module root, so the default works regardless of the caller's cwd
// (cmd/refs is invoked both via `go run ./cmd/refs` from src/go and via
// `go test` from cmd/refs itself).
func defaultVerdictsPath() (string, error) {
	root, ok := wavref.ModuleRoot()
	if !ok {
		return "", fmt.Errorf("refs audition: cannot locate the module root (no go.mod above cwd)")
	}
	return filepath.Join(root, verdictsRel), nil
}

// appendVerdict writes one verdict as a single JSON line, appending to
// path without disturbing whatever is already there -- the whole point of
// "append immediately" is that an interrupted session loses nothing already
// recorded.
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

// recordVerdict persists v, SUPERSEDING any existing verdict for the same
// family rather than appending a second one.
//
// Re-auditioning is a first-class workflow: any change to the render or
// level-match pipeline invalidates the affected families' recorded judgements,
// and the only fix is to listen again. Appending would leave two rows for one
// family in a file that is the labelled ground truth for downstream metric
// validation, distinguishable only by line order.
//
// Note/Tags handling is the one subtlety. A note explains WHY a family sounded
// the way it did, so it survives a re-recording that reaches the SAME verdict
// and is dropped when the verdict CHANGES -- prose written about "no, it's too
// buzzy" must not silently reattach itself to a later "yes". A note supplied
// with the new verdict always wins.
//
// New families still append, so ordering follows first-audition order.
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

// readVerdicts reads every recorded verdict from path. A missing file
// reads as zero verdicts, not an error -- a fresh session hasn't created
// it yet.
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
	// Notes are free text and could run long; keep the scanner's buffer
	// generous rather than choking on a verbose line.
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

// pickFamily resolves which family the CLI operates on this invocation.
//
// An explicit instrument (family key) always wins, regardless of resume or
// sessionSkip -- asking for a family by name is an explicit override of
// both "skip what I've already answered" and "skip what I just skipped".
// Left empty, it auto-picks the first family in families' (already
// Key-sorted) order, excluding:
//   - already-answered families, when resume is true (persisted across
//     runs, via the verdicts file)
//   - families in sessionSkip (NOT persisted -- see cmdAudition's -skip
//     flag: the `ab-all` shell loop accumulates "s"-skipped families across
//     ITS OWN loop iterations and re-passes them each invocation, so "s"
//     moves on to the next family for the rest of THIS session without
//     writing anything to the verdicts file; a future run still offers a
//     skipped family again)
//
// found=false with a nil error means "nothing left to do", not a failure.
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

// parseSkipSet parses the -skip flag's comma-separated family-key list into
// a lookup set. Empty entries (from a leading/trailing/doubled comma, or an
// entirely empty flag) are ignored so the Makefile's accumulator can grow
// via straightforward string concatenation without fussy quoting.
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
