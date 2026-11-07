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

// A note that wraps the terminal destroys the column alignment that makes the report scannable.
const noteMaxLen = 60

// A registered instrument with NO valid reference: its document differs from its base's, so the base's clip is not
// its ground truth. The to-do list for refs adopt.
const noRefLabel = "NO REF"

// Borrowing the base's reference, legitimate only when the variant IS the base's sound; labelled so a borrowed clip is
// never taken for adopted ground truth.
const viaLabel = "via"

// The audit view: every family, its reference and the listener's last word. Presence is an explicit seam, so the
// rendering is testable without depending on the developer's disk.
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
		// Already classified as synthetic / needs-source: a recorded decision, not an oversight, and listing it as NO REF would
		// bury the ids adoption is for.
		if covered[id] || classified[id] {
			continue
		}
		base := baseOf(id, roster)
		// A variant inherits its base's classification: "no acoustic counterpart exists for a kick" is about the instrument,
		// so kick-snap and kick-lofi are synthetic kicks too.
		if base != "" && classified[base] {
			continue
		}
		m := rosterMember{ID: id, Base: base}
		// Borrowing only when the variant IS the base's sound; one with its own document would be judged against the wrong clip.
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

// recordVerdict supersedes by appending, so an older opinion can sit above the current one; a stale verdict is worse
// than none.
func latestVerdicts(vs []earVerdict) map[instrument.ID]earVerdict {
	out := make(map[instrument.ID]earVerdict, len(vs))
	for _, v := range vs {
		out[v.Family] = v
	}
	return out
}

// The raw verdict letter (refs audition's alphabet: "1"/"2"/"y" an option won, "p" partly, "n" none close), the
// winner, and the note.
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

// Percussion has no usable f0, and a measured 0 means "not measurable", so it prints as a dash.
func f0Label(r wavref.Ref) string {
	if r.Measured == nil || r.Measured.F0Hz <= 0 {
		return "f0=-"
	}
	return fmt.Sprintf("f0=%.1fHz", r.Measured.F0Hz)
}

// A member is auditioned against the family's SHARED reference, which need not cover it, so its own references show
// too: all of them, since two candidates on one instrument is a refs choose decision nobody has made.
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

// Counted, not listed: refs doctor already enumerates them, and listing would drown the families.
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

// From buildAuditionFamilies, the grouping refs audition uses, so the listing never disagrees with what make ab-all asks.
func (rep familiesReport) render(w io.Writer) error {
	fams, err := buildAuditionFamilies(rep.Cat)
	if err != nil {
		return err
	}
	allFams := fams
	// refs audition's own -families parser (same rejection, same valid keys), resolved through familyKeyFor first: you
	// audit by instrument id, and the family key is printed nowhere you would have seen it.
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

	// Over ALL families, not the filtered view, so a roster extra is attributed to its real family when the report is narrowed.
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

	// tabwriter pads the empty trailing cells member rows emit for alignment, so each line is right-trimmed on the way out.
	var table bytes.Buffer
	tw := tabwriter.NewWriter(&table, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "FAMILY / MEMBER\tREFERENCE\tSTATE\tF0\tVERDICT")
	instruments, audited, needRef := 0, 0, 0
	// A SET of ref ids: one unresolvable reference shared by three members is one thing to fetch.
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
			// A member the family reference does not cover gets its own references spelled out, and they reach the missing count:
			// a member's own reference is what refs adopt creates.
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

	// A registered instrument whose base is in no family must still appear; a view narrowed to one family leaves orphans out.
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
