//go:build !js

package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// session_record.go — the "ear session" corpus: a richer companion to
// ear_verdicts.jsonl, recording not just a single y/n/pick per family but a
// full multi-candidate audition (rating + relative verdict + perceptual-scale
// answers + free adjectives) per clip. `refs validate`'s preference test
// (scorePreferences) and the -sessions-table scale-agreement report both read
// this file; `prefPairsFromSessions`/`prefPairsFromVerdicts` turn both corpora
// into the same currency -- ordered (chosen, rejected) pairs -- so a single
// scorer can consume verdicts recorded before this format existed alongside
// sessions recorded after.

// sessionsRel is the ear-session corpus, relative to the Go module root --
// mirrors verdictsRel's resolution pattern (defaultVerdictsPath).
const sessionsRel = "internal/audio/testdata/ear_sessions.jsonl"

// sessionClip identifies one renderable clip: either the family's CURRENT
// shipped instrument, or a session candidate.
//
// Base names the family's base instrument for a candidate clip -- the render
// target is Base (falling back to ID when Base is empty, e.g. for Current,
// where ID already IS the instrument id) with Overrides applied on top. ID
// alone stays the human-facing label ("A"/"B" for a candidate, the
// instrument id for Current) so a session reads naturally without forcing
// every candidate to repeat the base instrument id as its own ID.
type sessionClip struct {
	ID        string             `json:"id"`
	Base      instrument.ID      `json:"base,omitempty"`
	RecipeSHA string             `json:"recipe_sha"`
	Overrides map[string]float64 `json:"overrides,omitempty"`
	// DocSHA identifies a WAV-clone candidate (clone.DocSHA: the document's
	// content hash with render.amplitude excluded); `clone adopt` ships a
	// document only when its newest row here answered Accept = "y".
	DocSHA string `json:"doc_sha,omitempty"`
}

// renderInst returns the instrument id this clip should actually be rendered
// as: Base when set, else ID.
func (c sessionClip) renderInst() instrument.ID {
	if c.Base != "" {
		return c.Base
	}
	return instrument.ID(c.ID) // a Current clip is labelled by its instrument
}

// sessionScale is one perceptual-axis question answered during a session,
// already centred and direction-normalized: -2..+2, where +2 always means
// "the synth reads HIGH on Axis" regardless of how the question itself was
// worded (an inverted question's answer has already been flipped before it
// reaches this struct).
type sessionScale struct {
	Q      string `json:"q"`
	Axis   string `json:"axis"`
	Answer int    `json:"answer"`
}

// sessionCandidate is one alternative rendering auditioned against the
// family's Current clip within an earSession.
type sessionCandidate struct {
	sessionClip
	Label      string         `json:"label"`
	Rating     int            `json:"rating"`
	VsCurrent  string         `json:"vs_current"` // "better"|"same"|"worse"
	Scales     []sessionScale `json:"scales,omitempty"`
	Adjectives []string       `json:"adjectives,omitempty"`
	Accept     string         `json:"accept,omitempty"` // a clone candidate: "y" ship it, "n" no, "p" partly
}

// earSession is one line of the sessions JSONL file: one listener's full
// audition of a family's candidates against its current shipped instrument
// and real reference.
type earSession struct {
	Session string        `json:"session"`
	Family  instrument.ID `json:"family"`
	// Mode is the round: "quick" (the word salad only) or "deep" (plus the
	// 1-5 word ratings). A quick record's missing Scales were never asked.
	// Absent in records from before the two rounds (they asked the family
	// questions where a family had them).
	Mode        string             `json:"mode,omitempty"`
	Reference   string             `json:"reference"`
	SemisFromA3 int                `json:"semis_from_a3"`
	DurSec      float64            `json:"dur_sec"`
	Current     sessionClip        `json:"current"`
	Candidates  []sessionCandidate `json:"candidates"`
	Note        string             `json:"note,omitempty"`
	// Ship is the listener's decision, INDEPENDENT of closeness to the
	// reference: which version should be the shipped instrument -- "C" keep
	// the current one, or a candidate id. A listener may prefer a synth that
	// is nothing like the reference (the snare), so this is recorded on its
	// own and is deliberately NOT fed into the closer-to-reference preference
	// test.
	Ship string `json:"ship,omitempty"`
}

// The two rounds of a session (earSession.Mode).
const (
	sessionModeQuick = "quick"
	sessionModeDeep  = "deep"
)

// defaultSessionsPath resolves the committed sessions file location from the
// module root, so the default works regardless of the caller's cwd -- mirrors
// defaultVerdictsPath/defaultOrganicRangePath.
func defaultSessionsPath() (string, error) {
	root, ok := wavref.ModuleRoot()
	if !ok {
		return "", fmt.Errorf("refs session: cannot locate the module root (no go.mod above cwd)")
	}
	return filepath.Join(root, sessionsRel), nil
}

// readSessions loads every recorded session from path. A missing file is not
// an error -- it means no sessions have been recorded yet -- and returns
// (nil, nil), the same convention readVerdicts/candidatesFor use for "nothing
// here yet" vs. "couldn't read it".
func readSessions(path string) ([]earSession, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []earSession
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var s earSession
		if err := json.Unmarshal(line, &s); err != nil {
			return nil, fmt.Errorf("refs session: %s: %w", path, err)
		}
		out = append(out, s)
	}
	return out, sc.Err()
}

// appendSession writes one session as a single JSON line, appending to path
// without disturbing whatever is already there.
func appendSession(path string, s earSession) error {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}

// prefPair is one ordered preference: the listener judged Chosen to sound
// better than Rejected, for the same family against the same reference.
// scorePreferences (validate.go) diagnoses both clips against RefID and, per
// descriptor, counts how often Chosen's Norm reads lower (closer to the
// reference) than Rejected's.
type prefPair struct {
	Family           instrument.ID
	RefID            string
	Chosen, Rejected sessionClip
}

// prefPairsFromVerdicts derives preference pairs from the numbered-pick
// verdicts already recorded in ear_verdicts.jsonl: a "1"/"2"/"3" verdict with
// a Chosen option states that Chosen beat every OTHER option in that family,
// so it becomes one pair per rejected alternative. "y"/"n"/"neither"
// verdicts carry no relative comparison and are skipped.
func prefPairsFromVerdicts(vs []earVerdict) []prefPair {
	var out []prefPair
	for _, v := range vs {
		if v.Chosen == "" || len(v.Options) < 2 {
			continue
		}
		if v.Verdict != "1" && v.Verdict != "2" && v.Verdict != "3" {
			continue
		}
		for _, o := range v.Options {
			if o != v.Chosen {
				out = append(out, prefPair{Family: v.Family, RefID: v.Reference, Chosen: sessionClip{ID: string(v.Chosen)}, Rejected: sessionClip{ID: string(o)}})
			}
		}
	}
	return out
}

// prefPairsFromSessions derives preference pairs from the richer session
// corpus: a candidate marked "better" than the family's current instrument
// yields (candidate, current); "worse" yields (current, candidate); and any
// two candidates within the same session whose ratings differ yield
// (higher-rated, lower-rated) regardless of their VsCurrent verdicts.
func prefPairsFromSessions(ss []earSession) []prefPair {
	var out []prefPair
	for _, s := range ss {
		for i, c := range s.Candidates {
			switch c.VsCurrent {
			case "better":
				out = append(out, prefPair{Family: s.Family, RefID: s.Reference, Chosen: c.sessionClip, Rejected: s.Current})
			case "worse":
				out = append(out, prefPair{Family: s.Family, RefID: s.Reference, Chosen: s.Current, Rejected: c.sessionClip})
			}
			for j := i + 1; j < len(s.Candidates); j++ {
				d := s.Candidates[j]
				switch {
				case c.Rating > d.Rating:
					out = append(out, prefPair{Family: s.Family, RefID: s.Reference, Chosen: c.sessionClip, Rejected: d.sessionClip})
				case d.Rating > c.Rating:
					out = append(out, prefPair{Family: s.Family, RefID: s.Reference, Chosen: d.sessionClip, Rejected: c.sessionClip})
				}
			}
		}
	}
	return out
}
