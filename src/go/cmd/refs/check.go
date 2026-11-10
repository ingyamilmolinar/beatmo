//go:build !js

package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// refs check — the corpus-dependent calibration gates, run ON DEMAND.
//
// Reference audio is local tuning material: it is never committed, and no
// `go test` may read it (a test whose verdict depends on what one machine has
// in ~/.beatmo/wav-refs passes or skips by accident of the checkout). The
// checks below USED to be tests for exactly the reason they still exist --
// the committed calibrations (thresholds.json, organic_range.json, the
// F0ReliableConfidence constant, the shipped audition candidates) are derived
// from the corpus, and only a re-run against the corpus can tell that one has
// gone stale. So they run here instead: `make refs-check` locally, and as an
// explicit CI step after the corpus is fetched.
//
// A check whose corpus is not present SKIPS loudly (never a silent pass); any
// failure makes the command exit non-zero.

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

// corpusCheck is one named gate. run fills the report; it returns an error only
// when the check itself could not run (a missing committed file, a decode
// failure) -- a calibration that disagrees is a report failure, not an error.
type corpusCheck struct {
	name string
	doc  string
	run  func(r *checkReport) error
	// wholeCorpus marks a calibration over every instrument ref, locally
	// adopted ones included: a fresh one means nothing while any is missing.
	wholeCorpus bool
}

var corpusChecks = []corpusCheck{
	{"thresholds", "testdata/thresholds.json reproduces from a fresh calibration", checkThresholdsGolden, false},
	{"organic", "testdata/organic_range.json reproduces from a fresh calibration", checkOrganicRangeGolden, false},
	{"f0", "F0ReliableConfidence separates the corpus's pitched and unpitched clips", checkF0ConfidenceSeparation, false},
	{"f0-outliers", "the documented low-confidence pitched clips are still low", checkF0LowConfidenceOutliers, false},
	{"candidates", "every shipped audition candidate passes the pre-audition gate", checkCandidateGate, false},
	{"match-hubness", matchHubnessRel + " covers the roster and reproduces from a fresh calibration", checkMatchHubness, true},
}

var errCheckFailed = errors.New("refs check: one or more checks failed")

// corpusPresent counts the instrument-tier catalogue refs whose audio is on
// this machine and lists the fetchable ones that are not.
func corpusPresent(load func() (wavref.Cat, error)) (int, []string, error) {
	c, err := load()
	if err != nil {
		return 0, nil, err
	}
	n := 0
	var missing []string
	for _, r := range c.Refs {
		if r.Tier != wavref.TierInstrument || r.Status != "" || r.File == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(wavref.Root(), r.File)); err == nil {
			n++
		} else {
			missing = append(missing, r.ID)
		}
	}
	return n, missing, nil
}

func runChecks(w io.Writer, only map[string]bool, verbose bool) error {
	return runChecksFrom(w, only, verbose, loadCatalogue)
}

func runChecksFrom(w io.Writer, only map[string]bool, verbose bool, load func() (wavref.Cat, error)) error {
	n, missing, err := corpusPresent(load)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "reference corpus: %s (%d instrument refs present, %d missing)\n", wavref.Root(), n, len(missing))
	failed := false
	for _, c := range corpusChecks {
		if len(only) > 0 && !only[c.name] {
			continue
		}
		r := &checkReport{name: c.name}
		switch {
		case n == 0:
			r.Skipf("no reference audio present -- fetch it with `make fetch-refs TIER=instrument` or point %s at it", wavref.EnvVar)
		case c.wholeCorpus && len(missing) > 0:
			r.Skipf("calibrated over the whole corpus; missing here: %s", strings.Join(missing, ", "))
		default:
			if err := c.run(r); err != nil {
				r.Errorf("%v", err)
			}
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
	// Diagnosis writes its comparison audio under wavref.Dir(), which honours
	// only an EXPLICIT $BEATMO_WAV_REFS. Pin it to the root the corpus already
	// resolves from, so a plain `refs check` (default ~/.beatmo/wav-refs) runs
	// every check instead of failing the ones that diagnose.
	if os.Getenv(wavref.EnvVar) == "" {
		if err := os.Setenv(wavref.EnvVar, wavref.Root()); err != nil {
			return err
		}
	}
	return runChecks(os.Stdout, sel, *verbose)
}
