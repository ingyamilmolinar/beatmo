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

// Sentinels rather than os.Exit, so the dispatch stays unit-testable; main() maps them to the exit codes the ab-all
// recipe branches on.
var (
	errAuditionQuit   = errors.New("refs audition: quit requested")
	errAuditionReplay = errors.New("refs audition: replay requested")
)

// 1 is the generic error code; these are private to the ab-all shell loop.
const (
	exitAuditionReplay = 4
	exitAuditionQuit   = 3
)

// Pure metadata, so plan mode (after rendering) and prompt mode (never renders) print the same header.
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

// Spelled out after every answer: a "1" on a single-option family means "sounds like the real thing", a much stronger
// claim than "option 1 is closest" on a multi-option one.
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

// formatAuditionSh is the whole hand-off to the Makefile (the tool never runs a player): shQuote'd assignments like
// refs compare -sh, safe to eval with any path.
func formatAuditionSh(fam auditionFamily, results []compareResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "BEATMO_AB_FAMILY=%s\n", shQuote(string(fam.Key)))
	fmt.Fprintf(&b, "BEATMO_AB_REF=%s\n", shQuote(fam.Reference.ID))
	// The ONE shared reference every member was level-matched against.
	fmt.Fprintf(&b, "BEATMO_AB_REAL=%s\n", shQuote(results[0].RealPath))
	fmt.Fprintf(&b, "BEATMO_AB_DUR=%s\n", shQuote(fmt.Sprintf("%.2f", results[0].Dur)))
	fmt.Fprintf(&b, "BEATMO_AB_N=%s\n", shQuote(fmt.Sprintf("%d", len(fam.Members))))
	for i, m := range fam.Members {
		fmt.Fprintf(&b, "BEATMO_AB_OPT%d=%s\n", i+1, shQuote(string(m)))
		fmt.Fprintf(&b, "BEATMO_AB_OPT%d_PATH=%s\n", i+1, shQuote(results[i].SynthPath))
	}
	return b.String()
}

// The shell loop's termination signal (everything answered, or an empty catalogue).
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

// runAuditionPlan renders the reference and every member's option, level-matched, through refs compare's path, and
// prints the eval-able plan. The header is printed once, by runAuditionPrompt; this never reads stdin or records.
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

	// ONE family-wide render: the reference's level-match gain depends on its paired synth, so per-member renders made N
	// differently scaled copies of which only the first was played.
	results, err := renderFamilyCompare(diag, fam.Members, fam.Reference, 0, matchRMS)
	if err != nil {
		return err
	}

	fmt.Fprint(w, formatAuditionSh(fam, results))
	return nil
}

// runAuditionPrompt picks the same family without rendering (the header and menu are catalogue metadata), reads one
// answer and records a real verdict at once.
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
	// Unbuffered: ab-all runs a process per family on one piped stdin, and a read-ahead would strand the next answer.
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
