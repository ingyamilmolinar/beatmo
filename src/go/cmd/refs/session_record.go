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

// Per clip: rating, relative verdict, scale answers, words. Both this and ear_verdicts.jsonl become ordered
// (chosen, rejected) pairs, so one scorer reads verdicts and sessions alike.

// sessionsRel is the ear-session corpus, relative to the Go module root --
// mirrors verdictsRel's resolution pattern (defaultVerdictsPath).
const sessionsRel = "internal/audio/testdata/ear_sessions.jsonl"

// The render target is Base (else ID, as for Current) with Overrides on top; ID stays the human label ("A", or the
// instrument id for Current).
type sessionClip struct {
	ID        string             `json:"id"`
	Base      instrument.ID      `json:"base,omitempty"`
	RecipeSHA string             `json:"recipe_sha"`
	Overrides map[string]float64 `json:"overrides,omitempty"`
	// clone.DocSHA (content hash without render.amplitude); clone adopt ships a document only on its newest Accept "y".
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

// Centred and direction-normalised, -2..+2: +2 always means "the synth reads HIGH on Axis", however the question was
// worded.
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
	// "quick" (word salad only) or "deep" (plus the word ratings): a quick record's missing Scales were never asked.
	// Absent on older records.
	Mode        string             `json:"mode,omitempty"`
	Reference   string             `json:"reference"`
	SemisFromA3 int                `json:"semis_from_a3"`
	DurSec      float64            `json:"dur_sec"`
	Current     sessionClip        `json:"current"`
	Candidates  []sessionCandidate `json:"candidates"`
	Note        string             `json:"note,omitempty"`
	// Which version should ship ("C" or a candidate id), INDEPENDENT of closeness: the listener may prefer a synth nothing
	// like the reference, so it never feeds the closer-to-reference preference test.
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

// A missing file is (nil, nil): nothing recorded yet, not a read failure.
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

// scorePreferences diagnoses both clips against RefID and counts, per descriptor, how often Chosen reads closer.
type prefPair struct {
	Family           instrument.ID
	RefID            string
	Chosen, Rejected sessionClip
}

// A numbered pick beat every other option, so it yields one pair per rejected alternative; y/n/neither carry no
// comparison.
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

// "better" → (candidate, current), "worse" → (current, candidate), and two candidates rated differently →
// (higher, lower), whatever their VsCurrent.
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
