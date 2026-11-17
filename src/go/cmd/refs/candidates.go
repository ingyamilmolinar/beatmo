//go:build !js

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"text/tabwriter"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// candidateDir is the review directory for one instrument's competing
// references. It lives under $BEATMO_WAV_REFS so review audio never lands
// in the tree.
func candidateDir(inst instrument.ID) (string, error) {
	base, ok := wavref.Dir()
	if !ok {
		return "", fmt.Errorf("set %s to a directory for candidate review audio", wavref.EnvVar)
	}
	d := filepath.Join(base, "candidates", string(inst))
	if err := os.MkdirAll(d, 0o755); err != nil {
		return "", err
	}
	return d, nil
}

// runCandidates fetches up to n competing references for inst into the
// review directory and prints them side by side for ear comparison.
func runCandidates(w io.Writer, inst instrument.ID, n int) error {
	c, err := loadCatalogue()
	if err != nil {
		return err
	}
	refs := c.ForInstrument(inst)
	var real []wavref.Ref
	for _, r := range refs {
		if r.Status == "" {
			real = append(real, r)
		}
	}
	if len(real) == 0 {
		return fmt.Errorf("refs candidates: no candidate references for instrument %q", inst)
	}
	sort.Slice(real, func(i, j int) bool { return real[i].ID < real[j].ID })
	if n > 0 && len(real) > n {
		real = real[:n]
	}

	dir, err := candidateDir(inst)
	if err != nil {
		return err
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tF0 (Hz)\tSEMIS\tDUR (s)\tPEAK\tRMS\tFILE")
	var measured []wavref.Ref
	for i := range real {
		r := real[i]
		p, ok := wavref.Resolve(r.ID)
		if !ok {
			// r is a local copy from the `real` slice (itself built by
			// value from c.ForInstrument), so fetchOne's in-place update
			// of *r (Measured/PCMSHA256) mutates only this disposable
			// local copy, never the loaded catalogue c or the package's
			// memoized singleton. runCandidates is read-only with respect
			// to the catalogue; it never calls writeCatalogue/catalogueWriter.
			if _, err := fetchOne(w, &r, false); err != nil {
				fmt.Fprintf(w, "FAIL %v\n", err)
				continue
			}
			p, ok = wavref.Resolve(r.ID)
			if !ok {
				continue
			}
		}
		review := filepath.Join(dir, r.ID+".wav")
		if err := copyFile(p, review); err != nil {
			return err
		}
		m, err := measureFile(review)
		if err != nil {
			return err
		}
		fmt.Fprintf(tw, "%s\t%.1f\t%+d\t%.2f\t%.3f\t%.3f\t%s\n",
			r.ID, m.F0Hz, m.SemisFromA3, m.DurationSec, m.Peak, m.RMS, review)
		r.Measured = &m
		measured = append(measured, r)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	printPitchSpreadWarning(w, measured)
	fmt.Fprintf(w, "\nListen to the files above, then run:\n  refs choose -instrument %s -id <winning-id>\n", inst)
	return nil
}

// pitchSpreadOctaveSemis is the threshold at which a pitch mismatch is
// escalated from "the comparison is compromised" to "these are likely
// different registers or instruments" — one octave.
const pitchSpreadOctaveSemis = 12

// pitchSpreadSemis returns the max-min spread, in semitones, across refs'
// measured offsets from A3. Refs with no Measured block are ignored (a
// spread can't be computed on a note that hasn't been detected yet). Zero
// or one contributing ref spreads 0.
func pitchSpreadSemis(refs []wavref.Ref) int {
	var lo, hi int
	have := false
	for _, r := range refs {
		if r.Measured == nil {
			continue
		}
		s := r.Measured.SemisFromA3
		if !have {
			lo, hi, have = s, s, true
			continue
		}
		if s < lo {
			lo = s
		}
		if s > hi {
			hi = s
		}
	}
	if !have {
		return 0
	}
	return hi - lo
}

// printPitchSpreadWarning flags candidates recorded at different pitches: an
// instrument's timbre genuinely changes across its range, so "which sounds
// more like a violin" is unanswerable when one candidate is A4 and the
// other D5. A non-zero spread means the ear comparison above cannot judge
// timbre; a spread of an octave or more means the candidates may not even
// be the same instrument.
func printPitchSpreadWarning(w io.Writer, refs []wavref.Ref) {
	spread := pitchSpreadSemis(refs)
	if spread == 0 {
		return
	}
	fmt.Fprintf(w, "\nWARNING: candidates are %d semitone(s) apart in pitch.\n", spread)
	fmt.Fprintln(w, "An instrument's timbre genuinely changes across its range, so a like-for-like")
	fmt.Fprintln(w, "timbre judgement is NOT possible here. Judge recording quality instead — room")
	fmt.Fprintln(w, "tone, mic distance, background noise, articulation clarity — or re-point one")
	fmt.Fprintln(w, "candidate at a note matching its rival before comparing timbre.")
	if spread >= pitchSpreadOctaveSemis {
		fmt.Fprintln(w, "This is an octave or more apart: the candidates are likely different registers")
		fmt.Fprintln(w, "or even different instruments, and any comparison here is close to meaningless.")
	}
}

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}

// runChoose persists its mutated catalogue through catalogueWriter, the
// package-level var declared alongside catalogueLoader in fetch.go. Both
// catalogue-mutating commands (runChoose here, runFetch there) share that
// one seam so a test can redirect either write path to a throwaway file
// instead of mutating the committed catalogue.json as a side effect of
// running `go test ./cmd/refs/`.
//
// runChoose promotes one candidate to be inst's reference. The losers stay
// in the catalogue as Alternatives so the decision is recorded, reversible,
// and available to SP2 as labelled data.
func runChoose(w io.Writer, inst instrument.ID, id string) error {
	c, err := loadCatalogue()
	if err != nil {
		return err
	}
	winner, ok := c.Ref(id)
	if !ok {
		return fmt.Errorf("refs choose: unknown ref %q", id)
	}
	serves := false
	for _, i := range winner.Instruments {
		if i == inst {
			serves = true
			break
		}
	}
	if !serves {
		return fmt.Errorf("refs choose: ref %q is not a reference for instrument %q", id, inst)
	}

	// repointInstrument owns the whole "winner takes the instrument, losers
	// are recorded as alternatives and dropped if they now serve nothing"
	// rule, shared with `refs adopt`. It treats c.Refs and every nested
	// Instruments slice as read-only, which matters here: c came from
	// wavref.Catalogue(), which returns the process-wide memoized singleton
	// by value, so an in-place edit would corrupt every later
	// Catalogue()/loadCatalogue() call in this process.
	nAlts := countAlternatives(c.Refs, inst, id)
	refs := repointInstrument(c.Refs, inst, id, repointReason(inst, id))

	c.Refs = refs
	if err := catalogueWriter(c); err != nil {
		return err
	}
	fmt.Fprintf(w, "chose %s as the reference for %s (%d alternative(s) recorded)\n", id, inst, nAlts)
	return nil
}

func servesInstrument(r wavref.Ref, inst instrument.ID) bool {
	for _, i := range r.Instruments {
		if i == inst {
			return true
		}
	}
	return false
}

func cmdCandidates(args []string) error {
	fs := flag.NewFlagSet("candidates", flag.ExitOnError)
	inst := fs.String("instrument", "", "instrument id to gather candidates for")
	n := fs.Int("n", 3, "maximum candidates to prepare")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *inst == "" {
		return fmt.Errorf("refs candidates: -instrument is required")
	}
	return runCandidates(os.Stdout, instrument.ID(*inst), *n)
}

func cmdChoose(args []string) error {
	fs := flag.NewFlagSet("choose", flag.ExitOnError)
	inst := fs.String("instrument", "", "instrument id")
	id := fs.String("id", "", "winning ref id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *inst == "" || *id == "" {
		return fmt.Errorf("refs choose: -instrument and -id are both required")
	}
	return runChoose(os.Stdout, instrument.ID(*inst), *id)
}
