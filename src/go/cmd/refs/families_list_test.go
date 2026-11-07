//go:build !js

package main

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// testRef builds a minimal real (non-status) instrument-tier ref.
func testRef(id string, f0 float64, insts ...instrument.ID) wavref.Ref {
	return wavref.Ref{
		ID: id, Tier: wavref.TierInstrument, Instruments: insts,
		File:     id + ".wav",
		Source:   &wavref.Source{Provider: "local", URL: "/tmp/" + id},
		Measured: &wavref.Measured{F0Hz: f0},
	}
}

// allPresent is a presence stub: the render must not reach the real
// filesystem, so every test injects its own answer.
func allPresent(wavref.Ref) string { return "present" }

// Hermetic: the roster is the catalogue's own instruments unless a test says otherwise.
func renderReport(t *testing.T, rep familiesReport) string {
	t.Helper()
	if rep.Roster == nil {
		seen := map[string]bool{}
		for _, r := range rep.Cat.Refs {
			for _, i := range r.Instruments {
				if !seen[string(i)] {
					seen[string(i)], rep.Roster = true, append(rep.Roster, i)
				}
			}
		}
	}
	if rep.SoundOf == nil {
		// Distinct recipe per instrument: nothing inherits by default.
		rep.SoundOf = func(id instrument.ID) string { return string("recipe-" + id) }
	}
	var b strings.Builder
	if err := rep.render(&b); err != nil {
		t.Fatalf("render: %v", err)
	}
	return b.String()
}

// line returns the single output line containing want, failing when there
// is not exactly one -- so an assertion can't pass on a coincidental
// substring match somewhere else in the report.
func line(t *testing.T, out, want string) string {
	t.Helper()
	var hits []string
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, want) {
			hits = append(hits, l)
		}
	}
	if len(hits) != 1 {
		t.Fatalf("want exactly one line containing %q, got %d:\n%s", want, len(hits), out)
	}
	return hits[0]
}

// Matching the field, not a substring, keeps "cello" off "cello-warm" and the family header.
func memberLine(t *testing.T, out string, inst instrument.ID) string {
	t.Helper()
	var hits []string
	for _, l := range strings.Split(out, "\n") {
		if !strings.HasPrefix(l, "  ") {
			continue
		}
		fields := strings.Fields(strings.TrimPrefix(strings.TrimSpace(l), chosenMarker))
		if len(fields) > 0 && instrument.ID(fields[0]) == inst {
			hits = append(hits, l)
		}
	}
	if len(hits) != 1 {
		t.Fatalf("want exactly one member line for %q, got %d:\n%s", inst, len(hits), out)
	}
	return hits[0]
}

// The headline case: members of one family are listed under the single
// reference they are auditioned against, and the family's recorded ear
// verdict is joined on so you can see at a glance what you thought of it.
func TestFamilies_ListsMembersUnderSharedReferenceWithVerdict(t *testing.T) {
	c := wavref.Cat{Refs: []wavref.Ref{testRef("cello-A3", 220.0, roster.Cello, roster.CelloWarm)}}
	v := []earVerdict{{
		Family: roster.Cello, Reference: "cello-A3", Options: []instrument.ID{roster.Cello, roster.CelloWarm},
		Verdict: "1", Chosen: roster.Cello, Note: "but still too airy and breathy",
	}}

	out := renderReport(t, familiesReport{Cat: c, Verdicts: v, Presence: allPresent})

	head := line(t, out, "cello-A3")
	for _, want := range []string{string(roster.Cello), "present", "220.0", "1", "but still too airy and breathy"} {
		if !strings.Contains(head, want) {
			t.Errorf("family line missing %q:\n  %s", want, head)
		}
	}
	if !strings.Contains(out, "cello-warm") {
		t.Errorf("member cello-warm not listed:\n%s", out)
	}
}

// The verdict names a CHOSEN member; the listing must mark which one it was,
// because "this family sounded OK" is useless without "...as which variant".
func TestFamilies_MarksTheChosenMember(t *testing.T) {
	c := wavref.Cat{Refs: []wavref.Ref{testRef("cello-A3", 220.0, roster.Cello, roster.CelloWarm)}}
	v := []earVerdict{{Family: roster.Cello, Verdict: "2", Chosen: roster.CelloWarm}}

	out := renderReport(t, familiesReport{Cat: c, Verdicts: v, Presence: allPresent})

	chosen := memberLine(t, out, roster.CelloWarm)
	if !strings.Contains(chosen, chosenMarker) {
		t.Errorf("chosen member cello-warm not marked with %q:\n  %s", chosenMarker, chosen)
	}
	if plain := memberLine(t, out, roster.Cello); strings.Contains(plain, chosenMarker) {
		t.Errorf("unchosen member cello carries the chosen marker:\n  %s", plain)
	}
}

// A family nobody has listened to yet is the single most actionable row in
// the report, so it must say so rather than render a blank column.
func TestFamilies_UnauditedFamilyIsLabelled(t *testing.T) {
	c := wavref.Cat{Refs: []wavref.Ref{testRef("oboe-A4", 440.0, roster.Oboe)}}

	out := renderReport(t, familiesReport{Cat: c, Presence: allPresent})

	if head := line(t, out, "oboe-A4"); !strings.Contains(head, "unaudited") {
		t.Errorf("family with no verdict not labelled unaudited:\n  %s", head)
	}
}

// recordVerdict supersedes a family's earlier row by appending, so the file
// can hold several rows for one family. The listing must show the LAST one
// -- showing a stale opinion is worse than showing none.
func TestFamilies_UsesTheLastVerdictForAFamily(t *testing.T) {
	c := wavref.Cat{Refs: []wavref.Ref{testRef("oboe-A4", 440.0, roster.Oboe)}}
	v := []earVerdict{
		{Family: roster.Oboe, Verdict: "n", Note: "nothing like it"},
		{Family: roster.Oboe, Verdict: "y", Chosen: roster.Oboe, Note: "fixed by the new seed"},
	}

	out := renderReport(t, familiesReport{Cat: c, Verdicts: v, Presence: allPresent})

	head := line(t, out, "oboe-A4")
	if !strings.Contains(head, "fixed by the new seed") {
		t.Errorf("latest verdict note not shown:\n  %s", head)
	}
	if strings.Contains(out, "nothing like it") {
		t.Errorf("superseded verdict note still shown:\n%s", out)
	}
}

// conga-tumba joins conga by name but has its OWN reference: it is judged against one clip and hashed against another.
func TestFamilies_ShowsAMembersOwnReferenceWhenItDiffers(t *testing.T) {
	c := wavref.Cat{Refs: []wavref.Ref{
		testRef("conga-alt", 0, roster.Conga, roster.CongaOpen),
		testRef("conga-tumba-vcsl", 0, roster.CongaTumba),
	}}

	out := renderReport(t, familiesReport{Cat: c, Presence: allPresent})

	if m := line(t, out, "conga-tumba-vcsl"); !strings.Contains(m, "conga-tumba") {
		t.Errorf("member with its own reference does not show it:\n  %s", m)
	}
	if m := memberLine(t, out, roster.CongaOpen); strings.Fields(strings.TrimSpace(m))[len(strings.Fields(strings.TrimSpace(m)))-1] != "conga-open" {
		t.Errorf("member sharing the family reference should not repeat it:\n  %s", m)
	}
}

// Audio that can't be resolved locally is the reason a `make ab` run will
// silently skip, so it has to be shouted, not implied by a blank.
func TestFamilies_FlagsAReferenceWhoseAudioIsMissing(t *testing.T) {
	c := wavref.Cat{Refs: []wavref.Ref{testRef("oboe-A4", 440.0, roster.Oboe)}}

	out := renderReport(t, familiesReport{
		Cat:      c,
		Presence: func(wavref.Ref) string { return "missing" },
	})

	if head := line(t, out, "oboe-A4"); !strings.Contains(head, "missing") {
		t.Errorf("unresolvable reference not flagged:\n  %s", head)
	}
	if !strings.Contains(out, "1 reference") || !strings.Contains(out, "missing") {
		t.Errorf("summary does not count the missing reference:\n%s", out)
	}
}

// -instrument narrows to one family; everything else must disappear, not
// merely be highlighted.
func TestFamilies_FilterSelectsOneFamily(t *testing.T) {
	c := wavref.Cat{Refs: []wavref.Ref{
		testRef("cello-A3", 220.0, roster.Cello),
		testRef("oboe-A4", 440.0, roster.Oboe),
	}}

	out := renderReport(t, familiesReport{Cat: c, Filter: "oboe", Presence: allPresent})

	if !strings.Contains(out, "oboe") {
		t.Errorf("filtered family missing:\n%s", out)
	}
	if strings.Contains(out, "cello") {
		t.Errorf("unfiltered family still listed:\n%s", out)
	}
}

func TestFamilies_FilterOnAnUnknownFamilyIsAnError(t *testing.T) {
	c := wavref.Cat{Refs: []wavref.Ref{testRef("cello-A3", 220.0, roster.Cello)}}
	rep := familiesReport{Cat: c, Filter: "tuba", Presence: allPresent}

	var b strings.Builder
	if err := rep.render(&b); err == nil {
		t.Fatal("render() accepted an unknown family filter")
	}
}

// Status refs carry no audio and so form no family, but "what is NOT
// auditable" is half of an audit -- they get counted in the summary.
func TestFamilies_SummaryCountsStatusInstruments(t *testing.T) {
	c := wavref.Cat{Refs: []wavref.Ref{
		testRef("cello-A3", 220.0, roster.Cello),
		{ID: "s1", Tier: wavref.TierInstrument, Instruments: []instrument.ID{"808-kick"},
			Status: wavref.StatusSynthetic, Note: "no acoustic counterpart"},
		{ID: "s2", Tier: wavref.TierInstrument, Instruments: []instrument.ID{"rim"},
			Status: wavref.StatusNeedsSource, Note: "no clean recording found"},
	}}

	out := renderReport(t, familiesReport{Cat: c, Presence: allPresent})

	for _, want := range []string{"synthetic 1", "needs-source 1"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "808-kick") {
		t.Errorf("status instruments should be summarised, not listed:\n%s", out)
	}
}

// Song-tier refs belong to templates, not instrument families, and must not
// leak into the instrument audit.
func TestFamilies_IgnoresSongTierRefs(t *testing.T) {
	c := wavref.Cat{Refs: []wavref.Ref{
		testRef("cello-A3", 220.0, roster.Cello),
		{ID: "canon-song", Tier: wavref.TierSong, Template: "pachelbel-canon",
			File: "canon.wav", Source: &wavref.Source{Provider: "imslp"}},
	}}

	out := renderReport(t, familiesReport{Cat: c, Presence: allPresent})

	if strings.Contains(out, "canon") {
		t.Errorf("song-tier ref leaked into the instrument family listing:\n%s", out)
	}
}

// You audit by instrument id ("why does conga-open sound wrong?"), not by
// family key -- and the family key is not printed anywhere you'd have seen
// it first. Naming any MEMBER must therefore select its family.
func TestFamilies_FilterAcceptsAMemberName(t *testing.T) {
	c := wavref.Cat{Refs: []wavref.Ref{
		testRef("conga-alt", 0, roster.Conga, roster.CongaOpen),
		testRef("cello-A3", 220.0, roster.Cello),
	}}

	out := renderReport(t, familiesReport{Cat: c, Filter: "conga-open", Presence: allPresent})

	if !strings.Contains(out, "conga-alt") {
		t.Errorf("naming a member did not select its family:\n%s", out)
	}
	if strings.Contains(out, "cello") {
		t.Errorf("unrelated family still listed:\n%s", out)
	}
}

// The shared audition reference need not cover every member, and every reference covering one must be listed: two
// candidates on one instrument is a refs choose decision nobody has made.
func TestFamilies_ListsEveryReferenceCoveringAMemberOutsideTheFamilyReference(t *testing.T) {
	c := wavref.Cat{Refs: []wavref.Ref{
		testRef("cello-A3", 220.0, roster.CelloWarm),
		testRef("cello-C2", 65.0, roster.Cello),
		testRef("cello-freesound-alt", 66.0, roster.Cello),
	}}

	out := renderReport(t, familiesReport{Cat: c, Presence: allPresent})

	m := memberLine(t, out, roster.Cello)
	for _, want := range []string{"cello-C2", "cello-freesound-alt"} {
		if !strings.Contains(m, want) {
			t.Errorf("member line does not list its own reference %q:\n  %s", want, m)
		}
	}
	if w := memberLine(t, out, roster.CelloWarm); strings.Contains(w, "cello-A3") {
		t.Errorf("member covered BY the family reference should not repeat it:\n  %s", w)
	}
}

// One real note runs past 300 characters; wrapping would destroy the scan view's alignment.
func TestFamilies_TruncatesALongVerdictNote(t *testing.T) {
	long := strings.Repeat("very ", 60) + "wrong"
	c := wavref.Cat{Refs: []wavref.Ref{testRef("harp-alt", 0, roster.Harp)}}
	v := []earVerdict{{Family: roster.Harp, Verdict: "n", Note: long}}

	out := renderReport(t, familiesReport{Cat: c, Verdicts: v, Presence: allPresent})

	head := line(t, out, "harp-alt")
	if len(head) > 160 {
		t.Errorf("family line is %d chars, want it truncated to a scannable width:\n  %s", len(head), head)
	}
	if !strings.Contains(head, "very very") {
		t.Errorf("truncated note kept none of the note:\n  %s", head)
	}
	if !strings.Contains(head, noteEllipsis) {
		t.Errorf("truncated note not marked with %q:\n  %s", noteEllipsis, head)
	}
}

// Family headers and member rows share the reference column, so a reader
// can run their eye straight down it. That only works if every row emits
// the same number of cells to the tabwriter.
func TestFamilies_ReferenceColumnAlignsAcrossHeaderAndMemberRows(t *testing.T) {
	c := wavref.Cat{Refs: []wavref.Ref{
		testRef("cello-A3", 220.0, roster.CelloWarm),
		testRef("cello-C2", 65.0, roster.Cello),
	}}

	out := renderReport(t, familiesReport{Cat: c, Presence: allPresent})

	head := line(t, out, "cello-A3")
	member := memberLine(t, out, roster.Cello)
	if got, want := strings.Index(member, "cello-C2"), strings.Index(head, "cello-A3"); got != want {
		t.Errorf("reference column misaligned: header at %d, member at %d\n  %s\n  %s",
			want, got, head, member)
	}
}

// Every other refs table (list, doctor, doctor -pitch) labels its columns;
// six unlabelled columns of ids, states and numbers are unreadable without
// it.
func TestFamilies_PrintsAColumnHeader(t *testing.T) {
	c := wavref.Cat{Refs: []wavref.Ref{testRef("cello-A3", 220.0, roster.Cello)}}

	out := renderReport(t, familiesReport{Cat: c, Presence: allPresent})

	head := strings.SplitN(out, "\n", 2)[0]
	for _, want := range []string{"FAMILY", "REFERENCE", "STATE", "F0", "VERDICT"} {
		if !strings.Contains(head, want) {
			t.Errorf("column header missing %q:\n  %s", want, head)
		}
	}
}

// Member rows emit empty trailing cells to hold the column alignment, and
// tabwriter pads them -- leaving trailing spaces on most lines of a report
// people will pipe to a file and paste around.
func TestFamilies_EmitsNoTrailingWhitespace(t *testing.T) {
	c := wavref.Cat{Refs: []wavref.Ref{testRef("cello-A3", 220.0, roster.Cello, roster.CelloWarm)}}

	out := renderReport(t, familiesReport{Cat: c, Presence: allPresent})

	for i, l := range strings.Split(out, "\n") {
		if l != strings.TrimRight(l, " \t") {
			t.Errorf("line %d has trailing whitespace: %q", i+1, l)
		}
	}
}

// Driven by the GAME's roster, not by what catalogue.json mentions: enumerating the catalogue kept unmentioned
// registered ids invisible.
func TestFamilies_ListsRegisteredInstrumentsTheCatalogueNeverMentions(t *testing.T) {
	c := wavref.Cat{Refs: []wavref.Ref{testRef("snare-vcsl", 0, roster.Snare)}}

	out := renderReport(t, familiesReport{
		Cat:      c,
		Roster:   []instrument.ID{roster.Snare, roster.SnareCrack, roster.SnareLofi},
		SoundOf:  func(id instrument.ID) string { return string("recipe-" + id) }, // all distinct
		Presence: allPresent,
	})

	for _, id := range []instrument.ID{roster.SnareCrack, roster.SnareLofi} {
		if !strings.Contains(out, string(id)) {
			t.Errorf("registered instrument %q missing from the report:\n%s", id, out)
		}
	}
}

// A variant with its OWN document is a different sound: the base's clip is not its ground truth.
func TestFamilies_FlagsADistinctRecipeVariantAsNeedingItsOwnReference(t *testing.T) {
	c := wavref.Cat{Refs: []wavref.Ref{testRef("snare-vcsl", 0, roster.Snare)}}

	out := renderReport(t, familiesReport{
		Cat:    c,
		Roster: []instrument.ID{roster.Snare, roster.SnareLofi},
		SoundOf: func(id instrument.ID) string {
			if id == roster.SnareLofi {
				return "drum-snare-punchy" // its own binding
			}
			return "synth-modular-snare"
		},
		Presence: allPresent,
	})

	if m := memberLine(t, out, roster.SnareLofi); !strings.Contains(m, noRefLabel) {
		t.Errorf("distinct-recipe variant not flagged %q:\n  %s", noRefLabel, m)
	}
}

// A variant with the base's document IS the base's sound, so it borrows legitimately, and the report says so.
func TestFamilies_ShowsASameRecipeVariantAsBorrowingTheBaseReference(t *testing.T) {
	c := wavref.Cat{Refs: []wavref.Ref{testRef("organ-vcsl", 0, roster.Organ)}}

	out := renderReport(t, familiesReport{
		Cat:      c,
		Roster:   []instrument.ID{roster.Organ, "organ-2"},
		SoundOf:  func(instrument.ID) string { return "modular-organ" }, // both the same
		Presence: allPresent,
	})

	m := memberLine(t, out, "organ-2")
	if !strings.Contains(m, viaLabel) {
		t.Errorf("same-recipe variant not marked %q:\n  %s", viaLabel, m)
	}
	if !strings.Contains(m, "organ") {
		t.Errorf("borrowed reference does not name the base:\n  %s", m)
	}
	if strings.Contains(m, noRefLabel) {
		t.Errorf("same-recipe variant wrongly flagged as needing its own reference:\n  %s", m)
	}
}

// The summary has to count the work outstanding, or the report is a list
// rather than an audit.
func TestFamilies_SummaryCountsInstrumentsNeedingTheirOwnReference(t *testing.T) {
	c := wavref.Cat{Refs: []wavref.Ref{testRef("snare-vcsl", 0, roster.Snare)}}

	out := renderReport(t, familiesReport{
		Cat:      c,
		Roster:   []instrument.ID{roster.Snare, roster.SnareCrack, roster.SnareLofi},
		SoundOf:  func(id instrument.ID) string { return string("recipe-" + id) },
		Presence: allPresent,
	})

	if !strings.Contains(out, "2 need their own reference") {
		t.Errorf("summary does not count instruments needing a reference:\n%s", out)
	}
}

// A registered instrument whose base is in no family at all still has to
// appear somewhere -- silently dropping it is the bug this report fixes.
func TestFamilies_ListsARegisteredInstrumentWithNoFamilyAtAll(t *testing.T) {
	c := wavref.Cat{Refs: []wavref.Ref{testRef("snare-vcsl", 0, roster.Snare)}}

	out := renderReport(t, familiesReport{
		Cat:      c,
		Roster:   []instrument.ID{roster.Snare, "didgeridoo"},
		SoundOf:  func(id instrument.ID) string { return string("recipe-" + id) },
		Presence: allPresent,
	})

	if !strings.Contains(out, "didgeridoo") {
		t.Errorf("orphan registered instrument missing from the report:\n%s", out)
	}
}

// A variant of a status-classified base inherits the classification, or the to-do list fills with work nobody
// intends to do.
func TestFamilies_AVariantOfAStatusClassifiedBaseIsNotATodo(t *testing.T) {
	c := wavref.Cat{Refs: []wavref.Ref{
		testRef("snare-vcsl", 0, roster.Snare),
		{ID: "fm-family-synthetic", Tier: wavref.TierInstrument,
			Instruments: []instrument.ID{roster.Kick}, Status: wavref.StatusSynthetic,
			Note: "no acoustic counterpart"},
	}}

	out := renderReport(t, familiesReport{
		Cat:      c,
		Roster:   []instrument.ID{roster.Snare, roster.Kick, roster.KickSnap, roster.KickLofi},
		SoundOf:  func(id instrument.ID) string { return string("recipe-" + id) }, // all distinct
		Presence: allPresent,
	})

	for _, id := range []instrument.ID{roster.KickSnap, roster.KickLofi} {
		if strings.Contains(out, string(id)) {
			t.Errorf("%s is a variant of a synthetic base and must not be listed as a to-do:\n%s", id, out)
		}
	}
	if !strings.Contains(out, "0 need their own reference") {
		t.Errorf("summary counts synthetic-base variants as needing a reference:\n%s", out)
	}
}

// Narrowing to one family must show ONLY that family. Orphans belong to no
// family, so they must not tag along on every filtered view.
func TestFamilies_FilteredViewOmitsOrphans(t *testing.T) {
	c := wavref.Cat{Refs: []wavref.Ref{
		testRef("snare-vcsl", 0, roster.Snare),
		testRef("cello-A3", 220.0, roster.Cello),
	}}

	out := renderReport(t, familiesReport{
		Cat:      c,
		Filter:   "snare",
		Roster:   []instrument.ID{roster.Snare, roster.Cello, "didgeridoo"},
		SoundOf:  func(id instrument.ID) string { return string("recipe-" + id) },
		Presence: allPresent,
	})

	if strings.Contains(out, "didgeridoo") {
		t.Errorf("orphan leaked into a filtered view:\n%s", out)
	}
	if strings.Contains(out, "no family") {
		t.Errorf("the orphan block rendered in a filtered view:\n%s", out)
	}
}

// The summary must agree with the rows: a member's OWN missing reference (what refs adopt creates) must count, or the
// summary says nothing needs fetching while make ab is about to skip.
func TestFamilies_SummaryCountsAMissingMemberReference(t *testing.T) {
	c := wavref.Cat{Refs: []wavref.Ref{
		testRef("cello-A3", 220.0, roster.CelloWarm),
		testRef("cello-local", 65.0, roster.Cello),
	}}

	out := renderReport(t, familiesReport{
		Cat: c,
		Presence: func(r wavref.Ref) string {
			if r.ID == "cello-local" {
				return "missing"
			}
			return "present"
		},
	})

	if m := memberLine(t, out, roster.Cello); !strings.Contains(m, "missing") {
		t.Fatalf("member row does not flag its unresolvable reference:\n  %s", m)
	}
	if !strings.Contains(out, "1 reference missing locally") {
		t.Errorf("summary disagrees with the member row:\n%s", out)
	}
}
