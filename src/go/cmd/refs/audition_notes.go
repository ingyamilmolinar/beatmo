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

// Single-option families record y / p / n as is. Picking option "1".."N" is COMPARATIVE, not proof the instrument
// sounds right, so it classifies as "p": worth a note on what is still wrong with the winner.
func classifyVerdict(fam auditionFamily, v earVerdict) string {
	if len(fam.Members) > 1 {
		if v.Verdict == "n" {
			return "n"
		}
		return "p"
	}
	return v.Verdict
}

// Anything outside {y, n, p} is rejected, so a typo fails loudly instead of selecting nothing.
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

// The notes pass only revisits families with a verdict: there is nothing yet to explain otherwise.
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

// An explicit -instrument wins over -only and the already-noted check but still needs a recorded verdict; auto-pick
// walks pendingNoteCandidates, skipping the session's -skip list.
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

// The failure vocabulary from this project's tuning history; multi-word entries match as whole phrases, and the order is
// display order.
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

// No recognised vocabulary returns nil, which is fine: the note itself is always stored verbatim.
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

// A copy with that family's Note and Tags set, never a duplicate row; a family with no entry is an error by name.
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

// Atomic (temp file in the SAME directory, fsync, rename): these are irreplaceable human judgements, so a failure
// leaves path untouched.
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

// notesPromptLine: what was decided, the failure vocabulary and the controls. The WHOLE line is note prose; there is no
// choice token to parse out.
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

// The whole line is note prose unless it is exactly r / s / q; tags are parsed, the note is kept verbatim. Unbuffered
// (readLine): one process per family shares the piped stdin.
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

// runNotesPlan renders exactly what was judged (the verdict pass's render path) and reports how many families are
// pending, so the listener sees the scope up front.
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

	// One family-wide render: per-member renders re-derive the reference's gain from their own synth.
	results, err := renderFamilyCompare(diag, fam.Members, fam.Reference, 0, matchRMS)
	if err != nil {
		return err
	}
	fmt.Fprint(w, formatAuditionSh(fam, results))
	return nil
}

// runNotesPrompt: no rendering; reads one answer and updates the existing row in place.
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
