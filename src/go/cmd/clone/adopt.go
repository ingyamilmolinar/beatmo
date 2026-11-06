package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/ingyamilmolinar/beatmo/internal/audio/clone"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/schema"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// adopt.go — `clone adopt -inst <id>`: the ONLY writer of a cloned document
// into the shipped instruments, and only behind a recorded `y` (spec §6a).
// The verdict is keyed by the candidate's doc_sha (render.amplitude
// excluded), so a session that rated a different candidate cannot ship this
// one, and measure-loudness rewriting the amplitude afterwards changes
// nothing.

const sessionsRel = "internal/audio/testdata/ear_sessions.jsonl"

// sessionAccept is the part of an ear_sessions.jsonl row adopt reads (the
// full record lives in cmd/refs/session_record.go).
type sessionAccept struct {
	Session    string `json:"session"`
	Family     string `json:"family"`
	Candidates []struct {
		ID     string `json:"id"`
		DocSHA string `json:"doc_sha"`
		Accept string `json:"accept"`
	} `json:"candidates"`
}

func runAdopt(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("clone adopt", flag.ContinueOnError)
	fs.SetOutput(stderr)
	inst := fs.String("inst", "", "the instrument whose clone to adopt (its clones/<inst>/ directory)")
	kind := fs.String("kind", "clones", "candidate kind: clones (`make clone`) or tuned (`make tune`, $BEATMO_WAV_REFS/tuned/<inst>)")
	dirFlag := fs.String("dir", "", "candidate directory (default $BEATMO_WAV_REFS/<kind>/<inst>)")
	sessFlag := fs.String("sessions", "", "ear sessions JSONL (default "+sessionsRel+")")
	repoFlag := fs.String("repo", "", "module root to write into (default: the one above the cwd)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *inst == "" {
		fmt.Fprintln(stderr, "clone adopt: -inst is required")
		return 1
	}
	makeCandidate, listen := "make clone INST=%s", "make ab-session FAMILIES=%s CLONE=1"
	switch *kind {
	case "clones":
	case "tuned":
		makeCandidate, listen = "make tune INST=%s", "make ab-tuned FAMILIES=%s"
	default:
		fmt.Fprintf(stderr, "clone adopt: -kind %q: want clones or tuned\n", *kind)
		return 1
	}
	root := *repoFlag
	if root == "" {
		r, ok := wavref.ModuleRoot()
		if !ok {
			fmt.Fprintln(stderr, "clone adopt: cannot locate the module root")
			return 1
		}
		root = r
	}
	dir := *dirFlag
	if dir == "" {
		dir = filepath.Join(wavref.Root(), *kind, *inst)
	}
	sessions := *sessFlag
	if sessions == "" {
		sessions = filepath.Join(root, sessionsRel)
	}
	b, err := os.ReadFile(filepath.Join(dir, "candidate.json"))
	if err != nil {
		fmt.Fprintf(stderr, "clone adopt: no candidate for %s (run `"+makeCandidate+"`): %v\n", *inst, *inst, err)
		return 1
	}
	doc, err := instrument.ParseFile(b)
	if err != nil {
		fmt.Fprintln(stderr, "clone adopt:", err)
		return 1
	}
	sha := clone.DocSHA(doc)
	verdict, when, err := latestAccept(sessions, sha)
	if err != nil {
		fmt.Fprintln(stderr, "clone adopt:", err)
		return 1
	}
	switch verdict {
	case "":
		fmt.Fprintf(stderr, "clone adopt: no listening verdict for this candidate (doc_sha %s) — run `"+listen+"` first\n", sha, *inst)
		return 2
	case "y":
	default:
		fmt.Fprintf(stderr, "clone adopt: the newest session (%s) answered %q for this candidate — only a y ships it\n", when, verdict)
		return 2
	}
	dst := filepath.Join(root, "internal/assets/instruments", string(doc.ID)+".json")
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		fmt.Fprintln(stderr, "clone adopt:", err)
		return 1
	}
	added, err := addToIndex(filepath.Join(root, "internal/assets/instruments/index.json"), doc.ID)
	if err != nil {
		fmt.Fprintln(stderr, "clone adopt:", err)
		return 1
	}
	fmt.Fprintf(stdout, "adopted %s (session %s, accept y) → %s\n", doc.ID, when, dst)
	if added {
		fmt.Fprintf(stdout, "  added %s to index.json (roster order: last)\n", doc.ID)
	}
	fmt.Fprintln(stdout, "next: make asset-pack && (cd src/go && ../../.tools/go/bin/go generate ./internal/audio/) && make measure-loudness && make gen-fundamentals")
	return 0
}

// latestAccept returns the accept answer of the newest session holding a
// candidate with this doc_sha ("" when none does).
func latestAccept(path, sha string) (accept, when string, err error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return "", "", nil
	}
	if err != nil {
		return "", "", err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var s sessionAccept
		if json.Unmarshal(sc.Bytes(), &s) != nil {
			continue
		}
		for _, c := range s.Candidates {
			if c.DocSHA == sha && s.Session >= when {
				accept, when = c.Accept, s.Session
			}
		}
	}
	return accept, when, sc.Err()
}

// addToIndex appends id to the roster when it is not already there.
func addToIndex(path string, id instrument.ID) (bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	var idx struct {
		schema.Envelope
		Instruments []instrument.ID `json:"instruments"`
	}
	if err := json.Unmarshal(b, &idx); err != nil {
		return false, fmt.Errorf("%s: %w", path, err)
	}
	if slices.Contains(idx.Instruments, id) {
		return false, nil
	}
	idx.Instruments = append(idx.Instruments, id)
	out, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return false, err
	}
	return true, os.WriteFile(path, append(out, '\n'), 0o644)
}
