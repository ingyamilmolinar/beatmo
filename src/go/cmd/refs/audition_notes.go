//go:build !js

package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// --- Classification --------------------------------------------------

// classifyVerdict maps a recorded earVerdict onto one of "y" / "p" / "n"
// for -only filtering. Single-option families already record exactly one
// of those three tokens in Verdict, so classification is the identity.
// Multi-option families never record an absolute "y" -- "n" means
// "neither option is close" (kept as "n"), and picking a specific option
// ("1".."N") is a COMPARATIVE judgement, not proof the instrument sounds
// right, so it classifies as "p" (partly): worth a note explaining what's
// still wrong with even the winning option.
func classifyVerdict(fam auditionFamily, v earVerdict) string {
	if len(fam.Members) > 1 {
		if v.Verdict == "n" {
			return "n"
		}
		return "p"
	}
	return v.Verdict
}

// parseOnlySet parses the -only flag ("n", "p", or "n,p") into a lookup
// set, rejecting anything outside {y, n, p} so a typo fails loudly instead
// of silently selecting nothing.
func parseOnlySet(raw string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, tok := range strings.Split(raw, ",") {
		tok = strings.ToLower(strings.TrimSpace(tok))
		if tok == "" {
			continue
		}
		switch tok {
		case "y", "n", "p":
			out[tok] = true
		default:
			return nil, fmt.Errorf("refs audition -notes: -only %q: want y, n, and/or p (comma-separated)", tok)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("refs audition -notes: -only must name at least one of y, n, p")
	}
	return out, nil
}

// --- Selection ---------------------------------------------------------

// noteCandidate pairs a family with its already-recorded verdict, for the
// notes pass -- unlike the verdict pass, notes never operates on a family
// with no recorded verdict at all (there is nothing yet to explain).
type noteCandidate struct {
	Family  auditionFamily
	Verdict earVerdict
}

// pendingNoteCandidates returns, in family-Key order, every family that
// (a) has a recorded verdict, (b) classifies into `only`, and (c) either
// has no note yet or reannotate is true.
func pendingNoteCandidates(families []auditionFamily, verdicts []earVerdict, only map[string]bool, reannotate bool) []noteCandidate {
	byFamily := make(map[string]earVerdict, len(verdicts))
	for _, v := range verdicts {
		byFamily[string(v.Family)] = v
	}
	var out []noteCandidate
	for _, fam := range families {
		v, ok := byFamily[string(fam.Key)]
		if !ok {
			continue
		}
		if !only[classifyVerdict(fam, v)] {
			continue
		}
		if v.Note != "" && !reannotate {
			continue
		}
		out = append(out, noteCandidate{Family: fam, Verdict: v})
	}
	return out
}

// pickNotesFamily resolves which family THIS invocation revisits for
// notes, mirroring pickFamily's shape: an explicit -instrument always
// wins, bypassing -only and the already-noted check (asking for a family
// by name is an explicit override), but it still requires a recorded
// verdict to exist -- notes cannot explain a family that was never
// audited. Auto-pick walks pendingNoteCandidates in order, skipping
// sessionSkip (the -skip flag's non-persisted "s" accumulator, same
// convention as the verdict pass).
func pickNotesFamily(families []auditionFamily, verdicts []earVerdict, inst instrument.ID, only map[string]bool, reannotate bool, sessionSkip map[instrument.ID]bool) (auditionFamily, earVerdict, bool, error) {
	if inst != "" {
		var fam auditionFamily
		famFound := false
		for _, f := range families {
			if f.Key == inst {
				fam, famFound = f, true
				break
			}
		}
		if !famFound {
			return auditionFamily{}, earVerdict{}, false, fmt.Errorf("refs audition -notes: %q is not a known family (see `refs audition -list`)", inst)
		}
		for _, v := range verdicts {
			if v.Family == inst {
				return fam, v, true, nil
			}
		}
		return auditionFamily{}, earVerdict{}, false, fmt.Errorf("refs audition -notes: %q has no recorded verdict yet -- run `refs audition -inst %s` first", inst, inst)
	}

	for _, c := range pendingNoteCandidates(families, verdicts, only, reannotate) {
		if sessionSkip[c.Family.Key] {
			continue
		}
		return c.Family, c.Verdict, true, nil
	}
	return auditionFamily{}, earVerdict{}, false, nil
}

// --- Tag vocabulary ------------------------------------------------------

// recognizedNoteTags is the failure-vocabulary menu shown to the listener,
// drawn from this project's own tuning history (see
// docs/superpowers/specs/2026-08-21-ear-verdict-round-1-findings.md and
// reference_instrument_synth_tuning). Longer/multi-word entries are
// matched as whole phrases; order here is display order, not match
// priority.
var recognizedNoteTags = []string{
	"attack", "sustain", "bright", "dark", "nasal", "thin", "boomy",
	"noisy", "too clean", "decay", "vibrato", "metallic", "synthetic", "ensemble",
}

// noteTagPatterns compiles a word-boundary regexp per tag once, so e.g.
// "thin" never false-matches inside "nothing".
var noteTagPatterns = func() map[string]*regexp.Regexp {
	m := make(map[string]*regexp.Regexp, len(recognizedNoteTags))
	for _, t := range recognizedNoteTags {
		m[t] = regexp.MustCompile(`\b` + regexp.QuoteMeta(t) + `\b`)
	}
	return m
}()

// parseNoteTags extracts every recognized tag present in note, in order of
// first appearance, deduplicated. Free text with no recognized vocabulary
// returns an empty (nil) slice -- that's a perfectly valid outcome, not an
// error; the note itself is always stored verbatim by the caller
// regardless of what this returns.
func parseNoteTags(note string) []string {
	normalized := strings.ToLower(note)
	type hit struct {
		tag string
		pos int
	}
	var hits []hit
	for _, tag := range recognizedNoteTags {
		if loc := noteTagPatterns[tag].FindStringIndex(normalized); loc != nil {
			hits = append(hits, hit{tag, loc[0]})
		}
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].pos < hits[j].pos })
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.tag)
	}
	return out
}

// --- In-place update -----------------------------------------------------

// updateVerdictNote returns a COPY of verdicts with the matching family's
// Note and Tags set, preserving every other field and every other row
// untouched -- never appending a duplicate. A family with no existing
// entry is an error (there is nothing to attach the note to), reported by
// name rather than silently doing nothing or crashing.
func updateVerdictNote(verdicts []earVerdict, family instrument.ID, note string, tags []string) ([]earVerdict, error) {
	out := make([]earVerdict, len(verdicts))
	copy(out, verdicts)
	for i := range out {
		if out[i].Family == family {
			out[i].Note = note
			out[i].Tags = tags
			return out, nil
		}
	}
	return nil, fmt.Errorf("refs audition -notes: no recorded verdict for family %q -- nothing to annotate", family)
}

// --- Atomic write ----------------------------------------------------------

// writeVerdictsAtomic rewrites path's entire contents as one JSON line per
// verdict, atomically: it writes to a temp file in the SAME directory
// (so the final rename is same-filesystem and therefore atomic), fsyncs,
// then renames over path. This file holds irreplaceable human judgements,
// so a partial/truncating write is not an acceptable failure mode -- any
// error along the way removes the temp file and leaves path untouched.
func writeVerdictsAtomic(path string, verdicts []earVerdict) (err error) {
	dir := filepath.Dir(path)
	if dir != "" {
		if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
			return fmt.Errorf("refs audition -notes: mkdir %s: %w", dir, mkErr)
		}
	}

	tmp, err := os.CreateTemp(dir, ".ear_verdicts-*.tmp")
	if err != nil {
		return fmt.Errorf("refs audition -notes: create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		if err != nil {
			os.Remove(tmpPath)
		}
	}()

	bw := bufio.NewWriter(tmp)
	for _, v := range verdicts {
		b, mErr := marshalVerdict(v)
		if mErr != nil {
			tmp.Close()
			return fmt.Errorf("refs audition -notes: marshal verdict for %s: %w", v.Family, mErr)
		}
		if _, wErr := bw.Write(append(b, '\n')); wErr != nil {
			tmp.Close()
			return fmt.Errorf("refs audition -notes: write temp file: %w", wErr)
		}
	}
	if flErr := bw.Flush(); flErr != nil {
		tmp.Close()
		return fmt.Errorf("refs audition -notes: flush temp file: %w", flErr)
	}
	if syErr := tmp.Sync(); syErr != nil {
		tmp.Close()
		return fmt.Errorf("refs audition -notes: sync temp file: %w", syErr)
	}
	if clErr := tmp.Close(); clErr != nil {
		return fmt.Errorf("refs audition -notes: close temp file: %w", clErr)
	}

	// Preserve the original file's permission bits when it already exists,
	// so a rewrite doesn't quietly loosen/tighten permissions.
	mode := os.FileMode(0o644)
	if fi, statErr := os.Stat(path); statErr == nil {
		mode = fi.Mode()
	}
	if chErr := os.Chmod(tmpPath, mode); chErr != nil {
		return fmt.Errorf("refs audition -notes: chmod temp file: %w", chErr)
	}

	if rnErr := os.Rename(tmpPath, path); rnErr != nil {
		return fmt.Errorf("refs audition -notes: rename temp file into place: %w", rnErr)
	}
	return nil
}

// applyNoteToFile reads every verdict from path, updates the family's note
// in place, and rewrites the file atomically. On any error (including "no
// such family") the file on disk is left completely untouched.
func applyNoteToFile(path string, family instrument.ID, note string, tags []string) error {
	verdicts, err := readVerdicts(path)
	if err != nil {
		return err
	}
	updated, err := updateVerdictNote(verdicts, family, note, tags)
	if err != nil {
		return err
	}
	return writeVerdictsAtomic(path, updated)
}

// --- Prompt --------------------------------------------------------------

// describeVerdict renders a short human phrase for the verdict already on
// record, shown at the top of the notes prompt so the listener isn't
// asked to remember what they answered.
func describeVerdict(fam auditionFamily, v earVerdict) string {
	if len(fam.Members) > 1 {
		if v.Verdict == "n" {
			return "neither option is close"
		}
		return fmt.Sprintf("%s is closest", v.Chosen)
	}
	switch v.Verdict {
	case "y":
		return "sounds right"
	case "p":
		return "partly"
	case "n":
		return "does not sound right"
	}
	return v.Verdict
}

// notesPromptLine renders the notes-only prompt: what was already decided,
// the failure-vocabulary menu, and the control tokens. Unlike
// promptLine's choice+optional-note shape, the ENTIRE line here is note
// prose -- there is no separate choice token to parse out.
func notesPromptLine(fam auditionFamily, v earVerdict) string {
	var b strings.Builder
	fmt.Fprintf(&b, "notes for %s (recorded verdict: %s", fam.Key, describeVerdict(fam, v))
	if v.Note != "" {
		fmt.Fprintf(&b, "; existing note: %q", v.Note)
	}
	fmt.Fprint(&b, ")\n")
	fmt.Fprintf(&b, "tags: %s\n", strings.Join(recognizedNoteTags, ", "))
	fmt.Fprint(&b, "type any combination plus free text, e.g. \"sustain static, nasal, attack too soft\"\n")
	fmt.Fprint(&b, "r=replay  s=skip  q=quit\n> ")
	return b.String()
}

// promptNotes writes the prompt for fam/v to out, reads exactly one answer
// from in (re-prompting on blank input), and returns it. "r"/"s"/"q" as
// the WHOLE line (case-insensitive) are the control tokens; any other
// non-empty line is note prose in full -- there is no verdict to parse out
// here, unlike promptVerdict. Tags are parsed from the note but the note
// itself is always the verbatim, untouched text the listener typed.
//
// Reads via readLine (byte-at-a-time, unbuffered) for the SAME reason
// promptVerdict does: ab-notes runs one `refs audition -notes` process per
// family sharing one piped stdin, so this process must never read ahead
// into the next family's answer.
func promptNotes(in io.Reader, out io.Writer, fam auditionFamily, v earVerdict) (kind verdictKind, note string, tags []string, err error) {
	for {
		fmt.Fprint(out, notesPromptLine(fam, v))
		line, gotAny := readLine(in)
		if !gotAny {
			return verdictQuit, "", nil, nil
		}
		line = strings.TrimSpace(line)
		switch strings.ToLower(line) {
		case "q":
			return verdictQuit, "", nil, nil
		case "r":
			return verdictReplay, "", nil, nil
		case "s":
			return verdictSkipped, "", nil, nil
		case "":
			fmt.Fprintln(out, "invalid: empty input, try again (or r=replay, s=skip, q=quit)")
			continue
		}
		return verdictRecorded, line, parseNoteTags(line), nil
	}
}

// formatNoteConfirmation echoes exactly what was recorded, mirroring
// formatVerdictConfirmation's role for the verdict pass.
func formatNoteConfirmation(fam auditionFamily, note string, tags []string) string {
	tagStr := "none"
	if len(tags) > 0 {
		tagStr = strings.Join(tags, ", ")
	}
	return fmt.Sprintf("recorded note for %s: %q (tags: %s)", fam.Key, note, tagStr)
}

// --- Plan / prompt entry points -------------------------------------------

// runNotesPlan mirrors runAuditionPlan's shape for the notes pass: resolve
// the SAME family pickNotesFamily/runNotesPrompt would pick, render the
// reference plus every member's synth option (identical render path to
// the verdict pass -- notes replays exactly what was judged), and print
// the eval-able BEATMO_AB_* plan. It also reports how many families are
// pending before starting, so a listener sees the scope of the pass up
// front (see CLAUDE.md's "prompt for something specific" requirement).
func runNotesPlan(w, diag io.Writer, inst instrument.ID, only map[string]bool, reannotate bool, outPath, skip, familiesOnly string) error {
	c, err := loadCatalogue()
	if err != nil {
		return err
	}
	families, err := buildAuditionFamilies(c)
	if err != nil {
		return err
	}
	if families, err = filterFamilies(families, familiesOnly); err != nil {
		return err
	}
	verdicts, err := readVerdicts(outPath)
	if err != nil {
		return err
	}

	pending := pendingNoteCandidates(families, verdicts, only, reannotate)
	noun := "families"
	if len(pending) == 1 {
		noun = "family"
	}
	fmt.Fprintf(diag, "notes: %d %s to revisit\n", len(pending), noun)

	fam, _, found, err := pickNotesFamily(families, verdicts, inst, only, reannotate, parseSkipSet(skip))
	if err != nil {
		return err
	}
	if !found {
		fmt.Fprintln(diag, "no pending families left for notes -- nothing matches -only, or everything's annotated")
		fmt.Fprint(w, formatAuditionDone())
		return nil
	}

	// One family-wide render, for the same reason runAuditionPlan uses it:
	// per-member renderCompare calls each re-derive the reference's gain from
	// their own synth, and only the first member's copy is ever played.
	results, err := renderFamilyCompare(diag, fam.Members, fam.Reference, 0, matchRMS)
	if err != nil {
		return err
	}
	fmt.Fprint(w, formatAuditionSh(fam, results))
	return nil
}

// runNotesPrompt mirrors runAuditionPrompt's shape for the notes pass: no
// rendering, resolve the same family, print the header + notes prompt,
// read exactly one answer, and on a real note (not skip/replay/quit)
// update the existing verdict row in place via applyNoteToFile.
func runNotesPrompt(w io.Writer, in io.Reader, inst instrument.ID, only map[string]bool, reannotate bool, outPath, skip, familiesOnly string) (verdictKind, error) {
	c, err := loadCatalogue()
	if err != nil {
		return 0, err
	}
	families, err := buildAuditionFamilies(c)
	if err != nil {
		return 0, err
	}
	if families, err = filterFamilies(families, familiesOnly); err != nil {
		return 0, err
	}
	verdicts, err := readVerdicts(outPath)
	if err != nil {
		return 0, err
	}

	fam, v, found, err := pickNotesFamily(families, verdicts, inst, only, reannotate, parseSkipSet(skip))
	if err != nil {
		return 0, err
	}
	if !found {
		fmt.Fprintln(w, "no pending families left for notes -- nothing matches -only, or everything's annotated")
		return verdictSkipped, nil
	}

	printAuditionHeader(w, fam)
	kind, note, tags, err := promptNotes(in, w, fam, v)
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
		if err := applyNoteToFile(outPath, fam.Key, note, tags); err != nil {
			return 0, err
		}
		fmt.Fprintln(w, formatNoteConfirmation(fam, note, tags))
		return verdictRecorded, nil
	}
	return kind, nil
}
