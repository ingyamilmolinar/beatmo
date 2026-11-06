//go:build !js

package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// errAuditionQuit and errAuditionReplay are sentinel errors cmdAudition
// returns instead of calling os.Exit directly, so the dispatch logic stays
// unit-testable (killing the test process with os.Exit would defeat that).
// main() maps them to distinct process exit codes the `ab-all` Makefile
// recipe branches on.
var (
	errAuditionQuit   = errors.New("refs audition: quit requested")
	errAuditionReplay = errors.New("refs audition: replay requested")
)

// Process exit codes cmdAudition signals via the sentinel errors above.
// 1 is already used for generic errors (see main.go); these are additional,
// private to the ab-all shell loop.
const (
	exitAuditionReplay = 4
	exitAuditionQuit   = 3
)

// printAuditionHeader writes the compact per-family header: family name,
// reference id, measured pitch, and option count. Pure metadata -- no
// render required, so both plan mode (after rendering) and prompt mode
// (never renders) can print an identical header.
func printAuditionHeader(w io.Writer, fam auditionFamily) {
	n := len(fam.Members)
	noun := "option"
	if n != 1 {
		noun = "options"
	}
	semis, dur := 0, 0.0
	if fam.Reference.Measured != nil {
		semis = fam.Reference.Measured.SemisFromA3
		dur = fam.Reference.Measured.DurationSec
	}
	fmt.Fprintf(w, "\n=== %s (%d %s) === ref=%s pitch=%+d dur=%.2fs\n",
		fam.Key, n, noun, fam.Reference.ID, semis, dur)
	fmt.Fprintf(w, "    members: %s\n", strings.Join(instrument.Strings(fam.Members), ", "))
}

// formatVerdictConfirmation renders a plain-English statement of exactly
// what a recorded verdict means, so the user never has to guess what went
// into the JSONL -- critical because a stray "1" on a single-option family
// (accepted as a synonym for "y") records a MUCH stronger claim ("sounds
// like the real thing") than "1" means on a multi-option family ("option 1
// is closest"). Printed immediately after every accepted answer.
func formatVerdictConfirmation(fam auditionFamily, verdict string, chosen instrument.ID, note string) string {
	var msg string
	switch {
	case len(fam.Members) > 1 && verdict == "n":
		msg = "recorded: neither option is close"
	case len(fam.Members) > 1:
		msg = fmt.Sprintf("recorded: %s is closest", chosen)
	case verdict == "y":
		msg = fmt.Sprintf("recorded: %s sounds like the real thing", chosen)
	case verdict == "p":
		msg = fmt.Sprintf("recorded: %s partly sounds like the real thing", chosen)
	case verdict == "n":
		msg = fmt.Sprintf("recorded: %s does NOT sound like the real thing", fam.Members[0])
	}
	if note != "" {
		msg += fmt.Sprintf(" (note: %q)", note)
	}
	return msg
}

// formatAuditionSh renders the eval-able BEATMO_AB_* plan for one family --
// the reference clip plus one option per family member -- mirroring
// `refs compare -sh`'s shQuote'd assignment convention so `ab-all` can
// `eval` it safely even when a path contains a quote or a metacharacter. The Go tool never invokes a player itself; this is the entire
// hand-off to the Makefile.
func formatAuditionSh(fam auditionFamily, results []compareResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "BEATMO_AB_FAMILY=%s\n", shQuote(string(fam.Key)))
	fmt.Fprintf(&b, "BEATMO_AB_REF=%s\n", shQuote(fam.Reference.ID))
	// results[0].RealPath is now the ONE shared reference clip every member was
	// level-matched against (renderFamilyCompare writes exactly one), not
	// member 0's private copy.
	fmt.Fprintf(&b, "BEATMO_AB_REAL=%s\n", shQuote(results[0].RealPath))
	fmt.Fprintf(&b, "BEATMO_AB_DUR=%s\n", shQuote(fmt.Sprintf("%.2f", results[0].Dur)))
	fmt.Fprintf(&b, "BEATMO_AB_N=%s\n", shQuote(fmt.Sprintf("%d", len(fam.Members))))
	for i, m := range fam.Members {
		fmt.Fprintf(&b, "BEATMO_AB_OPT%d=%s\n", i+1, shQuote(string(m)))
		fmt.Fprintf(&b, "BEATMO_AB_OPT%d_PATH=%s\n", i+1, shQuote(results[i].SynthPath))
	}
	return b.String()
}

// formatAuditionDone is the sentinel `-sh` output when no family is left to
// audition (everything answered, or an empty catalogue) -- the shell loop's
// termination signal.
func formatAuditionDone() string {
	return "BEATMO_AB_DONE='1'\n"
}

// printResumeNote reports how many families -resume is skipping, once per
// invocation, so a resumed session isn't silent about why it jumped ahead.
func printResumeNote(w io.Writer, resume bool, families []auditionFamily, answered map[instrument.ID]bool) {
	if !resume || len(answered) == 0 {
		return
	}
	n := 0
	for _, f := range families {
		if answered[f.Key] {
			n++
		}
	}
	if n > 0 {
		fmt.Fprintf(w, "resume: %d of %d families already answered, skipping\n", n, len(families))
	}
}

// runAuditionPlan resolves the family to audition (explicit -instrument, or
// auto-picked per -resume/-skip) and renders the reference plus every member's
// synth option, level-matched, reusing computeCompare's renderCompare core
// (the SAME render/level-match path `refs compare` uses -- no duplicate
// logic). It prints render diagnostics (NOT the family header -- that's
// printed exactly once, by runAuditionPrompt, so the user doesn't see it
// twice across the two invocations) to diag, and the eval-able
// BEATMO_AB_* plan to w. It never touches stdin and never appends a
// verdict.
func runAuditionPlan(w, diag io.Writer, inst instrument.ID, resume bool, outPath, skip, only string) error {
	c, err := loadCatalogue()
	if err != nil {
		return err
	}
	families, err := buildAuditionFamilies(c)
	if err != nil {
		return err
	}
	if families, err = filterFamilies(families, only); err != nil {
		return err
	}
	answered, err := loadAnsweredFamilies(outPath)
	if err != nil {
		return err
	}
	printResumeNote(diag, resume, families, answered)

	fam, found, err := pickFamily(families, inst, resume, answered, parseSkipSet(skip))
	if err != nil {
		return err
	}
	if !found {
		fmt.Fprintln(diag, "no pending families left -- every family has a recorded verdict")
		fmt.Fprint(w, formatAuditionDone())
		return nil
	}

	// ONE renderFamilyCompare call for the whole family, never one
	// renderCompare per member: the reference's level-match gain depends on the
	// synth it is paired with, so per-member calls produced N differently-scaled
	// copies of the same reference while only the first was ever played. See
	// renderFamilyCompare's doc comment.
	results, err := renderFamilyCompare(diag, fam.Members, fam.Reference, 0, matchRMS)
	if err != nil {
		return err
	}

	fmt.Fprint(w, formatAuditionSh(fam, results))
	return nil
}

// runAuditionPrompt resolves the SAME family selection runAuditionPlan
// would (explicit -instrument, or auto-picked per -resume/-skip) but does no
// rendering at all -- everything it needs (family key, reference id,
// measured pitch, option list) is catalogue metadata. It prints the header
// and choice menu to w, reads exactly one answer from in, and on a real
// verdict (not skip/replay/quit) appends it to outPath immediately.
func runAuditionPrompt(w io.Writer, in io.Reader, family instrument.ID, resume bool, outPath, skip, only string) (verdictKind, error) {
	c, err := loadCatalogue()
	if err != nil {
		return 0, err
	}
	families, err := buildAuditionFamilies(c)
	if err != nil {
		return 0, err
	}
	if families, err = filterFamilies(families, only); err != nil {
		return 0, err
	}
	answered, err := loadAnsweredFamilies(outPath)
	if err != nil {
		return 0, err
	}

	fam, found, err := pickFamily(families, family, resume, answered, parseSkipSet(skip))
	if err != nil {
		return 0, err
	}
	if !found {
		fmt.Fprintln(w, "no pending families left -- every family has a recorded verdict")
		return verdictSkipped, nil
	}

	printAuditionHeader(w, fam)
	// in is read directly (NOT wrapped in a buffered reader) -- see
	// promptVerdict's doc comment: ab-all runs one process per family
	// sharing the same piped stdin, and a bufio read-ahead would silently
	// strand the next family's answer in a buffer that dies with this
	// process.
	kind, verdict, chosen, note, err := promptVerdict(in, w, fam)
	if err != nil {
		return 0, err
	}

	switch kind {
	case verdictQuit:
		return verdictQuit, nil
	case verdictReplay:
		return verdictReplay, nil
	case verdictSkipped:
		fmt.Fprintln(w, "skipped -- won't be offered again this session (see the -skip flag); a future run will still ask")
		return verdictSkipped, nil
	case verdictRecorded:
		semis := 0
		if fam.Reference.Measured != nil {
			semis = fam.Reference.Measured.SemisFromA3
		}
		v := earVerdict{
			Family:      fam.Key,
			Reference:   fam.Reference.ID,
			Options:     fam.Members,
			Verdict:     verdict,
			Chosen:      chosen,
			Note:        note,
			SemisFromA3: semis,
		}
		v.RecipeSHA = audio.InstrumentSoundHash(chosenMember(v), nil)
		// recordVerdict, not appendVerdict: re-auditioning a family must
		// supersede its previous judgement rather than leave two rows for it.
		if err := recordVerdict(outPath, v); err != nil {
			return 0, err
		}
		fmt.Fprintln(w, formatVerdictConfirmation(fam, verdict, chosen, note))
		return verdictRecorded, nil
	}
	return kind, nil
}

func cmdAudition(args []string) error {
	fs := flag.NewFlagSet("audition", flag.ExitOnError)
	inst := fs.String("instrument", "", "family base id to audition (default: auto-pick the next pending family)")
	out := fs.String("out", "", "verdicts jsonl path (default: internal/audio/testdata/ear_verdicts.jsonl)")
	resume := fs.Bool("resume", true, "skip families already recorded in the verdicts file (auto-pick only; an explicit -instrument always runs)")
	families := fs.String("families", "", "comma-separated family keys to restrict this session to (default: all); an unknown name is an error. Use to re-audition a targeted subset after a render or level-match change")
	skip := fs.String("skip", "", "comma-separated family keys to exclude from auto-pick for THIS invocation only (never persisted); the ab-all shell loop accumulates \"s\"-skipped families here across its own loop so skip advances within a session without writing anything to the verdicts file")
	sh := fs.Bool("sh", false, "plan mode: render + print machine-readable BEATMO_AB_* assignments; no prompt, no stdin")
	notes := fs.Bool("notes", false, "second-pass mode: revisit families that already have a recorded verdict and prompt for prose explaining WHY, in place -- never re-asks the verdict itself")
	only := fs.String("only", "n,p", "-notes only: comma-separated verdict classes to revisit (y, n, p)")
	reannotate := fs.Bool("reannotate", false, "-notes only: revisit families that already have a non-empty note (default: skip them)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	outPath := *out
	if outPath == "" {
		p, err := defaultVerdictsPath()
		if err != nil {
			return err
		}
		outPath = p
	}

	if *notes {
		onlySet, err := parseOnlySet(*only)
		if err != nil {
			return err
		}
		if *sh {
			return runNotesPlan(os.Stdout, os.Stderr, instrument.ID(*inst), onlySet, *reannotate, outPath, *skip, *families)
		}
		kind, err := runNotesPrompt(os.Stdout, os.Stdin, instrument.ID(*inst), onlySet, *reannotate, outPath, *skip, *families)
		if err != nil {
			return err
		}
		switch kind {
		case verdictQuit:
			return errAuditionQuit
		case verdictReplay:
			return errAuditionReplay
		}
		return nil
	}

	if *sh {
		return runAuditionPlan(os.Stdout, os.Stderr, instrument.ID(*inst), *resume, outPath, *skip, *families)
	}

	kind, err := runAuditionPrompt(os.Stdout, os.Stdin, instrument.ID(*inst), *resume, outPath, *skip, *families)
	if err != nil {
		return err
	}
	switch kind {
	case verdictQuit:
		return errAuditionQuit
	case verdictReplay:
		return errAuditionReplay
	}
	return nil
}
