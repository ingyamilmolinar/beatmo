//go:build !js

package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// The corpus-dependent calibration gates, run on demand: no go test may read reference audio, yet only a re-run against
// the corpus shows a committed calibration (thresholds, organic range, f0 confidence, audition candidates) has gone stale.
// A check without its corpus SKIPS loudly; any failure exits non-zero.

// checkReport collects one check's outcome. Errorf marks the check failed;
// Logf is detail printed under -v.
type checkReport struct {
	name    string
	failed  []string
	logs    []string
	skipped string
}

func (r *checkReport) Errorf(format string, args ...any) {
	r.failed = append(r.failed, fmt.Sprintf(format, args...))
}

func (r *checkReport) Logf(format string, args ...any) {
	r.logs = append(r.logs, fmt.Sprintf(format, args...))
}

func (r *checkReport) Skipf(format string, args ...any) {
	r.skipped = fmt.Sprintf(format, args...)
}

// An error only when the check could not run; a calibration that disagrees is a report failure.
type corpusCheck struct {
	name string
	doc  string
	run  func(r *checkReport) error
}

var corpusChecks = []corpusCheck{
	{"thresholds", "testdata/thresholds.json reproduces from a fresh calibration", checkThresholdsGolden},
	{"organic", "testdata/organic_range.json reproduces from a fresh calibration", checkOrganicRangeGolden},
	{"f0", "F0ReliableConfidence separates the corpus's pitched and unpitched clips", checkF0ConfidenceSeparation},
	{"f0-outliers", "the documented low-confidence pitched clips are still low", checkF0LowConfidenceOutliers},
	{"candidates", "every shipped audition candidate passes the pre-audition gate", checkCandidateGate},
	{"match-hubness", matchHubnessRel + " covers the roster and reproduces from a fresh calibration", checkMatchHubness},
}

var errCheckFailed = errors.New("refs check: one or more checks failed")

// corpusPresent reports how many instrument-tier catalogue refs resolve to
// audio on this machine.
func corpusPresent() (int, error) {
	c, err := loadCatalogue()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range c.Refs {
		if r.Tier != wavref.TierInstrument {
			continue
		}
		if _, ok := wavref.Resolve(r.ID); ok {
			n++
		}
	}
	return n, nil
}

func runChecks(w io.Writer, only map[string]bool, verbose bool) error {
	n, err := corpusPresent()
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "reference corpus: %s (%d instrument refs present)\n", wavref.Root(), n)
	failed := false
	for _, c := range corpusChecks {
		if len(only) > 0 && !only[c.name] {
			continue
		}
		r := &checkReport{name: c.name}
		if n == 0 {
			r.Skipf("no reference audio present -- fetch it with `make fetch-refs TIER=instrument` or point %s at it", wavref.EnvVar)
		} else if err := c.run(r); err != nil {
			r.Errorf("%v", err)
		}
		switch {
		case len(r.failed) > 0:
			failed = true
			fmt.Fprintf(w, "FAIL  %-12s %s\n", c.name, c.doc)
			for _, f := range r.failed {
				fmt.Fprintf(w, "        %s\n", strings.ReplaceAll(f, "\n", "\n        "))
			}
		case r.skipped != "":
			fmt.Fprintf(w, "SKIP  %-12s %s\n", c.name, r.skipped)
		default:
			fmt.Fprintf(w, "ok    %-12s %s\n", c.name, c.doc)
		}
		if verbose || len(r.failed) > 0 {
			for _, l := range r.logs {
				fmt.Fprintf(w, "        | %s\n", l)
			}
		}
	}
	if failed {
		return errCheckFailed
	}
	return nil
}

func cmdCheck(args []string) error {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	only := fs.String("only", "", "comma-separated subset of checks to run (default: all)")
	verbose := fs.Bool("v", false, "print every check's detail, not only a failing one's")
	if err := fs.Parse(args); err != nil {
		return err
	}
	sel := map[string]bool{}
	if *only != "" {
		known := map[string]bool{}
		var names []string
		for _, c := range corpusChecks {
			known[c.name] = true
			names = append(names, c.name)
		}
		sort.Strings(names)
		for _, n := range strings.Split(*only, ",") {
			n = strings.TrimSpace(n)
			if !known[n] {
				return fmt.Errorf("refs check: unknown check %q (have %s)", n, strings.Join(names, ", "))
			}
			sel[n] = true
		}
	}
	// wavref.Dir() honours only an EXPLICIT $BEATMO_WAV_REFS: pinned to the corpus root, so a plain refs check runs the
	// diagnosing checks too.
	if os.Getenv(wavref.EnvVar) == "" {
		if err := os.Setenv(wavref.EnvVar, wavref.Root()); err != nil {
			return err
		}
	}
	return runChecks(os.Stdout, sel, *verbose)
}
