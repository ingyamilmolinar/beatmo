//go:build !js

package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// chosenMarker flags the family member the listener actually picked in the
// recorded ear verdict. A family verdict without it ("this sounded fine")
// is unactionable -- you need to know WHICH variant sounded fine.
const chosenMarker = "*"

// noteEllipsis marks a verdict note the report had to cut short. The full
// prose lives in ear_verdicts.jsonl; this view is for scanning.
const noteEllipsis = "..."

// noteMaxLen is how much of a verdict note survives into the scan view.
// One recorded note runs past 300 characters; wrapping the terminal would
// destroy the column alignment that makes the report scannable at all.
const noteMaxLen = 60

// noRefLabel marks a registered instrument that has NO valid reference: it
// renders its own sound (its document differs from its base's), so its base
// instrument's clip is not ground truth for it. These are the audit's
// to-do list -- `refs adopt` is how each one gets a reference.
const noRefLabel = "NO REF"

// viaLabel marks a registered instrument BORROWING its base's reference.
// Legitimate only when the variant renders exactly the base's sound,
// i.e. it is the same sound; the label keeps a borrowed clip from being
// mistaken for ground truth adopted for that variant.
const viaLabel = "via"

// familiesReport renders the audit view of the corpus: every instrument
// family, the reference its members are auditioned against, and what the
// listener last said about it.
//
// It is a value with an explicit Presence seam rather than a function that
// calls wavref.Resolve directly, so the rendering is testable against a
// synthetic catalogue without depending on which audio happens to be on
// the developer's disk.
type familiesReport struct {
	Cat      wavref.Cat
	Verdicts []earVerdict
	// Filter narrows the report to one family. Empty lists every family.
	Filter string
	// Presence answers "is this ref's audio resolvable right now?".
	// nil defaults to the package-level presence().
	Presence func(wavref.Ref) string
	// Roster is the GAME's instrument registration -- the list the audit
	// measures the catalogue against. nil defaults to rosterIDs().
	Roster []instrument.ID
	// SoundOf identifies an instrument's shipped sound, used to decide
	// whether a variant may borrow its base's reference (same key = same
	// sound). nil defaults to audio.ShippedSoundKey.
	SoundOf func(instrument.ID) string
}

// rosterMember is a registered instrument with no reference of its own.
type rosterMember struct {
	ID instrument.ID
	// Base is the registered instrument it derives its name from, or "".
	Base instrument.ID
	// Borrows is true when it renders exactly Base's sound, so Base's
	// reference clip is valid ground truth for it.
	Borrows bool
}

// baseOf returns the longest OTHER roster id that id extends at a hyphen
// boundary ("snare-lofi" -> "snare", "fm-bell-soft" -> "fm-bell"), or "".
func baseOf(id instrument.ID, roster []instrument.ID) instrument.ID {
	var best instrument.ID
	for _, cand := range roster {
		if cand == id || !strings.HasPrefix(string(id), string(cand)+"-") {
			continue
		}
		if len(cand) > len(best) {
			best = cand
		}
	}
	return best
}

// rosterExtras classifies every registered instrument that no family member
// covers and no status ref has already accounted for, keyed by the family
// key it belongs under ("" for an orphan whose base is in no family).
func rosterExtras(roster []instrument.ID, covered, classified map[instrument.ID]bool, familyOf map[instrument.ID]instrument.ID, soundOf func(instrument.ID) string) map[instrument.ID][]rosterMember {
	out := map[instrument.ID][]rosterMember{}
	for _, id := range roster {
		// An instrument the catalogue has already classified as
		// synthetic or needs-source is deliberately without a
		// reference, not an oversight. Listing it as NO REF would bury
		// the genuinely unclassified ids -- the ones adoption is for --
		// under decisions that were already made and recorded.
		if covered[id] || classified[id] {
			continue
		}
		base := baseOf(id, roster)
		// A variant inherits its base's classification. "No acoustic
		// counterpart exists for a kick" is a statement about the
		// instrument, not about one recipe: kick-snap and kick-lofi are
		// synthetic kicks too, and listing them as work to do would
		// bury the ids that genuinely need a recording.
		if base != "" && classified[base] {
			continue
		}
		m := rosterMember{ID: id, Base: base}
		// Borrowing is only sound when the variant IS the base's sound.
		// A variant with its own document (snare-crack, kick-lofi) renders
		// differently, so judging it against the base's clip would
		// measure the wrong thing.
		if base != "" && soundOf(id) != "" && soundOf(id) == soundOf(base) {
			m.Borrows = true
		}
		out[familyOf[base]] = append(out[familyOf[base]], m)
	}
	for k := range out {
		sort.Slice(out[k], func(i, j int) bool { return out[k][i].ID < out[k][j].ID })
	}
	return out
}

// latestVerdicts indexes verdicts by family, keeping the LAST row for each.
// recordVerdict supersedes by appending, so an older opinion can still be
// sitting above the current one in the file; showing a stale verdict would
// be worse than showing none.
func latestVerdicts(vs []earVerdict) map[instrument.ID]earVerdict {
	out := make(map[instrument.ID]earVerdict, len(vs))
	for _, v := range vs {
		out[v.Family] = v
	}
	return out
}

// verdictSummary renders one family's recorded judgement for the header
// line: the raw verdict letter (the same alphabet `refs audition` records --
// "1"/"2"/"y" an option won, "p" partly, "n" none close), which member won,
// and the prose note if one was captured.
func verdictSummary(v earVerdict, ok bool) string {
	if !ok {
		return "unaudited"
	}
	s := "verdict=" + v.Verdict
	if v.Chosen != "" {
		s += " chose " + string(v.Chosen)
	}
	if v.Note != "" {
		s += "  " + quoteNote(truncateNote(v.Note))
	}
	return s
}

// truncateNote shortens a note to noteMaxLen runes, marking the cut. It
// counts RUNES rather than bytes so a note carrying non-ASCII can't be
// sliced through the middle of a character.
func truncateNote(s string) string {
	r := []rune(s)
	if len(r) <= noteMaxLen {
		return s
	}
	return strings.TrimRight(string(r[:noteMaxLen]), " ") + noteEllipsis
}

// quoteNote quotes a free-text note so a multi-word note can't be mistaken
// for further columns.
func quoteNote(s string) string { return `"` + s + `"` }

// f0Label renders a reference's measured fundamental. Percussion clips
// legitimately have no usable f0, and a measured 0 means "not measurable"
// rather than "DC" -- so it prints as a dash rather than 0.0Hz.
func f0Label(r wavref.Ref) string {
	if r.Measured == nil || r.Measured.F0Hz <= 0 {
		return "f0=-"
	}
	return fmt.Sprintf("f0=%.1fHz", r.Measured.F0Hz)
}

// ownReferences lists every real reference covering inst, sorted by id.
//
// A family member is auditioned against the family's SHARED reference, which
// need not cover it: cello-A3 covers only cello-warm, while "cello" itself
// is covered by cello-C2 and cello-freesound-alt. An audit has to show both
// halves of that -- the member is JUDGED against one clip and HASHED against
// another -- and it has to show ALL of them, because an instrument carrying
// two candidate references is a `refs choose` decision nobody has made yet.
func ownReferences(c wavref.Cat, inst instrument.ID) []wavref.Ref {
	var out []wavref.Ref
	for _, r := range c.ForInstrument(inst) {
		if r.Status == "" {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// coversInstrument reports whether r is ground truth for inst.
func coversInstrument(r wavref.Ref, inst instrument.ID) bool {
	for _, i := range r.Instruments {
		if i == inst {
			return true
		}
	}
	return false
}

// statusCounts tallies the instruments that form no family because their
// ref carries a status instead of audio. "What is NOT auditable" is half of
// an audit, but listing them here would drown the families -- refs doctor
// already enumerates them, so the report only counts them.
func statusCounts(c wavref.Cat) (synthetic, needsSource int) {
	for _, r := range c.Refs {
		switch r.Status {
		case wavref.StatusSynthetic:
			synthetic += len(r.Instruments)
		case wavref.StatusNeedsSource:
			needsSource += len(r.Instruments)
		}
	}
	return synthetic, needsSource
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// familyKeyFor maps any family MEMBER id to its family key, leaving
// anything else (already a key, or a typo) untouched so filterFamilies
// still owns the unknown-name error.
func familyKeyFor(fams []auditionFamily, name string) string {
	for _, f := range fams {
		for _, m := range f.Members {
			if string(m) == name {
				return string(f.Key)
			}
		}
	}
	return name
}

// render writes the report. Families come from buildAuditionFamilies -- the
// same grouping `refs audition` uses -- so the listing can never disagree
// with what `make ab-all` will actually ask you about.
func (rep familiesReport) render(w io.Writer) error {
	fams, err := buildAuditionFamilies(rep.Cat)
	if err != nil {
		return err
	}
	allFams := fams
	// Reuses `refs audition`'s own -families parser, so a typo is rejected
	// with the same message and the same list of valid keys rather than
	// silently rendering an empty report. The filter is resolved through
	// familyKeyFor first: you audit by instrument id, and the family key is
	// not printed anywhere you would have seen it before running this.
	if rep.Filter != "" {
		fams, err = filterFamilies(fams, familyKeyFor(fams, rep.Filter))
		if err != nil {
			return err
		}
	}

	present := rep.Presence
	if present == nil {
		present = presence
	}
	roster := rep.Roster
	if roster == nil {
		roster = rosterIDs()
	}
	soundOf := rep.SoundOf
	if soundOf == nil {
		soundOf = audio.ShippedSoundKey
	}
	byFamily := latestVerdicts(rep.Verdicts)

	// Every registered instrument a family already covers, plus which
	// family covers it -- computed over ALL families (not the filtered
	// view) so a roster extra is attributed to its real family even when
	// the report is narrowed to one.
	covered := map[instrument.ID]bool{}
	familyOf := map[instrument.ID]instrument.ID{}
	for _, f := range allFams {
		for _, m := range f.Members {
			covered[m], familyOf[m] = true, f.Key
		}
	}
	classified := map[instrument.ID]bool{}
	for _, r := range rep.Cat.Refs {
		if r.Status == "" {
			continue
		}
		for _, i := range r.Instruments {
			classified[i] = true
		}
	}
	extras := rosterExtras(roster, covered, classified, familyOf, soundOf)

	// tabwriter pads every cell, including the empty trailing ones member
	// rows emit to hold the column alignment. That leaves trailing spaces
	// on most lines of a report people pipe to a file, so the table is
	// buffered and each line right-trimmed on the way out.
	var table bytes.Buffer
	tw := tabwriter.NewWriter(&table, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "FAMILY / MEMBER\tREFERENCE\tSTATE\tF0\tVERDICT")
	instruments, audited, needRef := 0, 0, 0
	// Counted as a SET of reference ids, not a tally of rows. The summary
	// reports references, and one unresolvable reference shared by three
	// members is one thing to go and fetch, not three.
	missing := map[string]bool{}
	for _, fam := range fams {
		v, ok := byFamily[fam.Key]
		if ok {
			audited++
		}
		state := present(fam.Reference)
		if state != "present" && fam.Reference.ID != "" {
			missing[fam.Reference.ID] = true
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			fam.Key, fam.Reference.ID, state, f0Label(fam.Reference),
			verdictSummary(v, ok))

		for _, m := range fam.Members {
			instruments++
			marker := " "
			if ok && v.Chosen == m {
				marker = chosenMarker
			}
			// A member the family reference already covers repeats
			// nothing; one it does not gets its own reference(s)
			// spelled out.
			// A member's OWN reference -- which is what `refs adopt`
			// creates -- has to reach the summary too. Counting only the
			// family reference let a row print "missing" under a summary
			// claiming nothing was missing locally.
			var ids, states []string
			if !coversInstrument(fam.Reference, m) {
				for _, r := range ownReferences(rep.Cat, m) {
					st := present(r)
					ids, states = append(ids, r.ID), append(states, st)
					if st != "present" {
						missing[r.ID] = true
					}
				}
			}
			// Same cell count as the header row, so the reference,
			// state and f0 columns line up straight down the report.
			fmt.Fprintf(tw, "  %s %s\t%s\t%s\t\t\n",
				marker, m, strings.Join(ids, " "), strings.Join(states, " "))
		}

		for _, e := range extras[fam.Key] {
			instruments++
			ref, state := noRefLabel, ""
			if e.Borrows {
				ref, state = viaLabel+" "+string(e.Base), present(fam.Reference)
			} else {
				needRef++
			}
			fmt.Fprintf(tw, "    %s\t%s\t%s\t\t\n", e.ID, ref, state)
		}
	}

	// Registered instruments whose base belongs to no family at all still
	// have to appear: silently dropping one is the blind spot this report
	// exists to remove.
	// Orphans belong to no family, so a view narrowed to one family must
	// not drag them along.
	if orphans := extras[""]; len(orphans) > 0 && rep.Filter == "" {
		fmt.Fprintf(tw, "(no family)\t\t\t\t\n")
		for _, e := range orphans {
			instruments++
			needRef++
			fmt.Fprintf(tw, "    %s\t%s\t\t\t\n", e.ID, noRefLabel)
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	for _, l := range strings.Split(strings.TrimRight(table.String(), "\n"), "\n") {
		if _, err := fmt.Fprintln(w, strings.TrimRight(l, " \t")); err != nil {
			return err
		}
	}

	synthetic, needsSource := statusCounts(rep.Cat)
	fmt.Fprintf(w, "\nnot comparable: synthetic %d, needs-source %d (see `make refs-doctor`)\n",
		synthetic, needsSource)
	fmt.Fprintf(w, "%s / %s registered -- %d audited, %d unaudited, %s missing locally, %d need their own reference\n",
		plural(len(fams), "family", "families"),
		plural(instruments, "instrument", "instruments"),
		audited, len(fams)-audited,
		plural(len(missing), "reference", "references"), needRef)
	return nil
}

func cmdFamilies(args []string) error {
	fs := flag.NewFlagSet("families", flag.ExitOnError)
	inst := fs.String("instrument", "", "narrow the report to one family")
	if err := fs.Parse(args); err != nil {
		return err
	}
	c, err := loadCatalogue()
	if err != nil {
		return err
	}
	path, err := defaultVerdictsPath()
	if err != nil {
		return err
	}
	verdicts, err := readVerdicts(path)
	if err != nil {
		return err
	}
	return familiesReport{
		Cat:      c,
		Verdicts: verdicts,
		Filter:   strings.TrimSpace(*inst),
	}.render(os.Stdout)
}
