//go:build !js

package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// --- classifyVerdict / parseOnlySet -----------------------------------

func TestClassifyVerdict_SingleOptionMirrorsVerdictField(t *testing.T) {
	fam := singleFam()
	for _, v := range []string{"y", "p", "n"} {
		got := classifyVerdict(fam, earVerdict{Verdict: v})
		if got != v {
			t.Errorf("classifyVerdict(single, %q) = %q, want %q", v, got, v)
		}
	}
}

func TestClassifyVerdict_MultiOptionNeitherIsN(t *testing.T) {
	fam := multiFam()
	if got := classifyVerdict(fam, earVerdict{Verdict: "n"}); got != "n" {
		t.Errorf("classifyVerdict(multi, n) = %q, want n", got)
	}
}

func TestClassifyVerdict_MultiOptionNumericChoiceIsP(t *testing.T) {
	fam := multiFam()
	if got := classifyVerdict(fam, earVerdict{Verdict: "1"}); got != "p" {
		t.Errorf("classifyVerdict(multi, 1) = %q, want p (a comparative pick, not an absolute y)", got)
	}
	if got := classifyVerdict(fam, earVerdict{Verdict: "2"}); got != "p" {
		t.Errorf("classifyVerdict(multi, 2) = %q, want p", got)
	}
}

func TestParseOnlySet_DefaultNP(t *testing.T) {
	got, err := parseOnlySet("n,p")
	if err != nil {
		t.Fatalf("parseOnlySet: %v", err)
	}
	if len(got) != 2 || !got["n"] || !got["p"] {
		t.Errorf("parseOnlySet(n,p) = %v, want {n,p}", got)
	}
}

func TestParseOnlySet_SingleValue(t *testing.T) {
	got, err := parseOnlySet("n")
	if err != nil {
		t.Fatalf("parseOnlySet: %v", err)
	}
	if len(got) != 1 || !got["n"] {
		t.Errorf("parseOnlySet(n) = %v, want {n}", got)
	}
}

func TestParseOnlySet_RejectsUnknownToken(t *testing.T) {
	if _, err := parseOnlySet("n,bogus"); err == nil {
		t.Error("parseOnlySet accepted an unknown token")
	}
}

func TestParseOnlySet_RejectsEmpty(t *testing.T) {
	if _, err := parseOnlySet(""); err == nil {
		t.Error("parseOnlySet accepted an empty set")
	}
}

// --- pendingNoteCandidates / pickNotesFamily: selection ----------------

func famSet() []auditionFamily {
	return []auditionFamily{
		{Key: roster.Bassoon, Members: []instrument.ID{roster.Bassoon}},
		{Key: roster.Cello, Members: []instrument.ID{roster.Cello, roster.CelloWarm}},
		{Key: roster.Harp, Members: []instrument.ID{roster.Harp}},
		{Key: roster.Koto, Members: []instrument.ID{roster.Koto}},
	}
}

func TestPendingNoteCandidates_SelectsNAndPExcludesY(t *testing.T) {
	verdicts := []earVerdict{
		{Family: roster.Bassoon, Verdict: "p"},
		{Family: roster.Cello, Verdict: "1"}, // multi-option numeric -> classified p
		{Family: roster.Harp, Verdict: "n"},
		{Family: roster.Koto, Verdict: "y"},
	}
	only := map[string]bool{"n": true, "p": true}
	got := pendingNoteCandidates(famSet(), verdicts, only, false)
	var keys []instrument.ID
	for _, c := range got {
		keys = append(keys, c.Family.Key)
	}
	want := []instrument.ID{roster.Bassoon, roster.Cello, roster.Harp}
	if !reflect.DeepEqual(keys, want) {
		t.Errorf("pendingNoteCandidates keys = %v, want %v (koto/y excluded)", keys, want)
	}
}

func TestPendingNoteCandidates_OnlyNNarrowsToFailuresOnly(t *testing.T) {
	verdicts := []earVerdict{
		{Family: roster.Bassoon, Verdict: "p"},
		{Family: roster.Harp, Verdict: "n"},
		{Family: roster.Koto, Verdict: "y"},
	}
	only := map[string]bool{"n": true}
	got := pendingNoteCandidates(famSet(), verdicts, only, false)
	if len(got) != 1 || got[0].Family.Key != roster.Harp {
		t.Errorf("pendingNoteCandidates(-only n) = %+v, want just harp", got)
	}
}

func TestPendingNoteCandidates_SkipsFamiliesWithNoVerdict(t *testing.T) {
	verdicts := []earVerdict{{Family: roster.Bassoon, Verdict: "n"}}
	only := map[string]bool{"n": true, "p": true}
	got := pendingNoteCandidates(famSet(), verdicts, only, false)
	if len(got) != 1 || got[0].Family.Key != roster.Bassoon {
		t.Errorf("pendingNoteCandidates = %+v, want just bassoon (others have no recorded verdict at all)", got)
	}
}

func TestPendingNoteCandidates_SkipsAlreadyNotedUnlessReannotate(t *testing.T) {
	verdicts := []earVerdict{
		{Family: roster.Bassoon, Verdict: "n", Note: "already explained: nasal attack"},
		{Family: roster.Harp, Verdict: "n"},
	}
	only := map[string]bool{"n": true, "p": true}

	got := pendingNoteCandidates(famSet(), verdicts, only, false)
	if len(got) != 1 || got[0].Family.Key != roster.Harp {
		t.Errorf("without -reannotate, bassoon (already noted) must be excluded: got %+v", got)
	}

	got = pendingNoteCandidates(famSet(), verdicts, only, true)
	var keys []instrument.ID
	for _, c := range got {
		keys = append(keys, c.Family.Key)
	}
	want := []instrument.ID{roster.Bassoon, roster.Harp}
	if !reflect.DeepEqual(keys, want) {
		t.Errorf("with -reannotate, bassoon must be revisited too: got %v, want %v", keys, want)
	}
}

func TestPickNotesFamily_ExplicitInstrumentBypassesFilterAndNote(t *testing.T) {
	verdicts := []earVerdict{{Family: roster.Koto, Verdict: "y", Note: "already has a note"}}
	only := map[string]bool{"n": true, "p": true} // koto/y wouldn't normally match
	fam, v, found, err := pickNotesFamily(famSet(), verdicts, roster.Koto, only, false, nil)
	if err != nil {
		t.Fatalf("pickNotesFamily: %v", err)
	}
	if !found || fam.Key != roster.Koto || v.Verdict != "y" {
		t.Errorf("explicit -instrument should win regardless of -only/-reannotate: got (%+v, %+v, %v)", fam, v, found)
	}
}

func TestPickNotesFamily_ExplicitInstrumentWithNoVerdictErrors(t *testing.T) {
	_, _, _, err := pickNotesFamily(famSet(), nil, roster.Koto, map[string]bool{"n": true}, false, nil)
	if err == nil {
		t.Error("pickNotesFamily should error for an explicit family with no recorded verdict")
	}
}

func TestPickNotesFamily_UnknownExplicitInstrumentErrors(t *testing.T) {
	_, _, _, err := pickNotesFamily(famSet(), nil, "not-a-family", map[string]bool{"n": true}, false, nil)
	if err == nil {
		t.Error("pickNotesFamily should error for an unknown -instrument")
	}
}

func TestPickNotesFamily_SessionSkipMovesToNext(t *testing.T) {
	verdicts := []earVerdict{
		{Family: roster.Bassoon, Verdict: "n"},
		{Family: roster.Harp, Verdict: "n"},
	}
	only := map[string]bool{"n": true, "p": true}
	fam, _, found, err := pickNotesFamily(famSet(), verdicts, "", only, false, map[instrument.ID]bool{roster.Bassoon: true})
	if err != nil {
		t.Fatalf("pickNotesFamily: %v", err)
	}
	if !found || fam.Key != roster.Harp {
		t.Errorf("pickNotesFamily(sessionSkip=bassoon) = (%+v, %v), want harp", fam, found)
	}
}

func TestPickNotesFamily_NoneLeftReportsNotFound(t *testing.T) {
	_, _, found, err := pickNotesFamily(famSet(), nil, "", map[string]bool{"n": true, "p": true}, false, nil)
	if err != nil {
		t.Fatalf("pickNotesFamily: %v", err)
	}
	if found {
		t.Error("pickNotesFamily with no recorded verdicts at all should report not-found")
	}
}

// --- parseNoteTags -------------------------------------------------------

func TestParseNoteTags_RecognisesMultipleTagsInOrder(t *testing.T) {
	got := parseNoteTags("sustain static, nasal, attack too soft")
	want := []string{"sustain", "nasal", "attack"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseNoteTags = %v, want %v", got, want)
	}
}

func TestParseNoteTags_MultiWordTag(t *testing.T) {
	got := parseNoteTags("way too clean, no grit at all")
	if len(got) != 1 || got[0] != "too clean" {
		t.Errorf("parseNoteTags = %v, want [\"too clean\"]", got)
	}
}

func TestParseNoteTags_UnrecognisedWordsProduceNoTags(t *testing.T) {
	got := parseNoteTags("muddy and hollow, hard to describe")
	if len(got) != 0 {
		t.Errorf("parseNoteTags(no known vocabulary) = %v, want empty", got)
	}
}

func TestParseNoteTags_WordBoundaryAvoidsFalsePositive(t *testing.T) {
	// "nothing" contains the substring "thin" but must not match the
	// "thin" tag -- word-boundary matching only.
	got := parseNoteTags("nothing wrong here")
	if len(got) != 0 {
		t.Errorf("parseNoteTags(%q) = %v, want empty (no false positive on \"thin\")", "nothing wrong here", got)
	}
}

func TestParseNoteTags_FreeTextWithNoTagsStillValid(t *testing.T) {
	got := parseNoteTags("just doesn't feel real, hard to say why")
	if got == nil {
		return // nil vs empty slice are equally valid "no tags"
	}
	if len(got) != 0 {
		t.Errorf("parseNoteTags = %v, want empty", got)
	}
}

// --- updateVerdictNote: in-place update, preserves other fields --------

func TestUpdateVerdictNote_PreservesOtherFieldsNoDuplication(t *testing.T) {
	verdicts := []earVerdict{
		{Family: roster.Cello, Reference: "cello-A3", Options: []instrument.ID{roster.Cello, roster.CelloWarm}, Verdict: "1", Chosen: roster.Cello, SemisFromA3: 0},
		{Family: roster.Oboe, Reference: "oboe-A4", Options: []instrument.ID{roster.Oboe, roster.OboeFull}, Verdict: "n", SemisFromA3: 3},
	}
	got, err := updateVerdictNote(verdicts, roster.Oboe, "nasal, thin in the mids", []string{"nasal", "thin"})
	if err != nil {
		t.Fatalf("updateVerdictNote: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("updateVerdictNote produced %d rows, want 2 (no duplication)", len(got))
	}
	// cello untouched.
	if !reflect.DeepEqual(got[0], verdicts[0]) {
		t.Errorf("unrelated family cello was mutated: %+v", got[0])
	}
	oboe := got[1]
	if oboe.Note != "nasal, thin in the mids" {
		t.Errorf("oboe.Note = %q, want the new note", oboe.Note)
	}
	if !reflect.DeepEqual(oboe.Tags, []string{"nasal", "thin"}) {
		t.Errorf("oboe.Tags = %v, want [nasal thin]", oboe.Tags)
	}
	// every other field preserved.
	if oboe.Reference != "oboe-A4" || oboe.Verdict != "n" || oboe.SemisFromA3 != 3 {
		t.Errorf("oboe fields not preserved: %+v", oboe)
	}
	if len(oboe.Options) != 2 || oboe.Options[0] != roster.Oboe || oboe.Options[1] != roster.OboeFull {
		t.Errorf("oboe.Options not preserved: %v", oboe.Options)
	}
}

func TestUpdateVerdictNote_FamilyAbsentIsHandledSensibly(t *testing.T) {
	verdicts := []earVerdict{{Family: roster.Cello, Verdict: "1"}}
	_, err := updateVerdictNote(verdicts, "no-such-family", "note", nil)
	if err == nil {
		t.Fatal("updateVerdictNote should error for a family with no recorded verdict")
	}
	if !strings.Contains(err.Error(), "no-such-family") {
		t.Errorf("error should name the missing family, got: %v", err)
	}
}

// --- writeVerdictsAtomic: atomicity, no truncation, no leftover temp ---

func TestWriteVerdictsAtomic_RoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "verdicts.jsonl")
	verdicts := []earVerdict{
		{Family: roster.Cello, Verdict: "1"},
		{Family: roster.Oboe, Verdict: "n", Note: "nasal", Tags: []string{"nasal"}},
	}
	if err := writeVerdictsAtomic(path, verdicts); err != nil {
		t.Fatalf("writeVerdictsAtomic: %v", err)
	}
	got, err := readVerdicts(path)
	if err != nil {
		t.Fatalf("readVerdicts: %v", err)
	}
	if !reflect.DeepEqual(got, verdicts) {
		t.Errorf("round trip mismatch: got %+v, want %+v", got, verdicts)
	}
}

func TestWriteVerdictsAtomic_NoTempFileLeftBehind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "verdicts.jsonl")
	if err := writeVerdictsAtomic(path, []earVerdict{{Family: roster.Cello, Verdict: "1"}}); err != nil {
		t.Fatalf("writeVerdictsAtomic: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "verdicts.jsonl" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("directory contains %v, want exactly [verdicts.jsonl] (no leftover temp file)", names)
	}
}

func TestWriteVerdictsAtomic_OverwritesWithoutTruncationOnError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "verdicts.jsonl")
	original := []earVerdict{{Family: roster.Cello, Verdict: "1"}, {Family: roster.Oboe, Verdict: "n"}}
	if err := writeVerdictsAtomic(path, original); err != nil {
		t.Fatalf("seed writeVerdictsAtomic: %v", err)
	}

	updated := append(append([]earVerdict{}, original...), earVerdict{Family: roster.Harp, Verdict: "n"})
	if err := writeVerdictsAtomic(path, updated); err != nil {
		t.Fatalf("writeVerdictsAtomic: %v", err)
	}
	got, err := readVerdicts(path)
	if err != nil {
		t.Fatalf("readVerdicts: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("readVerdicts after overwrite = %d rows, want 3 (file was never left truncated)", len(got))
	}
}

// --- applyNoteToFile: end-to-end file update ----------------------------

func TestApplyNoteToFile_UpdatesInPlace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "verdicts.jsonl")
	seed := []earVerdict{
		{Family: roster.Cello, Reference: "cello-A3", Options: []instrument.ID{roster.Cello, roster.CelloWarm}, Verdict: "1", Chosen: roster.Cello},
		{Family: roster.Oboe, Reference: "oboe-A4", Options: []instrument.ID{roster.Oboe}, Verdict: "n"},
	}
	if err := writeVerdictsAtomic(path, seed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := applyNoteToFile(path, roster.Oboe, "nasal, thin", []string{"nasal", "thin"}); err != nil {
		t.Fatalf("applyNoteToFile: %v", err)
	}

	got, err := readVerdicts(path)
	if err != nil {
		t.Fatalf("readVerdicts: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("applyNoteToFile changed row count to %d, want 2", len(got))
	}
	if got[0].Family != roster.Cello || got[0].Note != "" {
		t.Errorf("cello row should be untouched: %+v", got[0])
	}
	if got[1].Family != roster.Oboe || got[1].Note != "nasal, thin" || !reflect.DeepEqual(got[1].Tags, []string{"nasal", "thin"}) {
		t.Errorf("oboe row not updated as expected: %+v", got[1])
	}
}

func TestApplyNoteToFile_AbsentFamilyErrorsWithoutWriting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "verdicts.jsonl")
	seed := []earVerdict{{Family: roster.Cello, Verdict: "1"}}
	if err := writeVerdictsAtomic(path, seed); err != nil {
		t.Fatalf("seed: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := applyNoteToFile(path, "no-such-family", "note", nil); err == nil {
		t.Fatal("applyNoteToFile should error for an unrecorded family")
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("applyNoteToFile must not modify the file when the update itself fails")
	}
}

// --- promptNotes: prompt loop over an injected io.Reader/io.Writer -----

func TestPromptNotes_RecordsNoteAndParsesTags(t *testing.T) {
	in := strings.NewReader("sustain static, nasal, attack too soft\n")
	var out bytes.Buffer
	kind, note, tags, err := promptNotes(in, &out, singleFam(), earVerdict{Verdict: "n"})
	if err != nil {
		t.Fatalf("promptNotes: %v", err)
	}
	if kind != verdictRecorded {
		t.Fatalf("kind = %v, want verdictRecorded", kind)
	}
	if note != "sustain static, nasal, attack too soft" {
		t.Errorf("note = %q, stored not verbatim", note)
	}
	want := []string{"sustain", "nasal", "attack"}
	if !reflect.DeepEqual(tags, want) {
		t.Errorf("tags = %v, want %v", tags, want)
	}
	if !strings.Contains(out.String(), "bassoon") {
		t.Errorf("prompt should mention the family, got:\n%s", out.String())
	}
}

func TestPromptNotes_EmptyInputReprompts(t *testing.T) {
	in := strings.NewReader("\nnasal\n")
	var out bytes.Buffer
	kind, note, tags, err := promptNotes(in, &out, singleFam(), earVerdict{Verdict: "n"})
	if err != nil {
		t.Fatalf("promptNotes: %v", err)
	}
	if kind != verdictRecorded || note != "nasal" || !reflect.DeepEqual(tags, []string{"nasal"}) {
		t.Errorf("got (%v, %q, %v), want recorded/nasal/[nasal] after re-prompt", kind, note, tags)
	}
	if !strings.Contains(out.String(), "invalid") {
		t.Errorf("expected a re-prompt message for empty input, got:\n%s", out.String())
	}
}

func TestPromptNotes_ReplaySkipQuit(t *testing.T) {
	cases := map[string]verdictKind{"r\n": verdictReplay, "s\n": verdictSkipped, "q\n": verdictQuit}
	for input, want := range cases {
		kind, _, _, err := promptNotes(strings.NewReader(input), &bytes.Buffer{}, singleFam(), earVerdict{Verdict: "n"})
		if err != nil {
			t.Fatalf("promptNotes(%q): %v", input, err)
		}
		if kind != want {
			t.Errorf("promptNotes(%q) = %v, want %v", input, kind, want)
		}
	}
}

func TestPromptNotes_FreeTextWithNoTagsStillStored(t *testing.T) {
	in := strings.NewReader("just doesn't feel real somehow\n")
	var out bytes.Buffer
	kind, note, tags, err := promptNotes(in, &out, singleFam(), earVerdict{Verdict: "n"})
	if err != nil {
		t.Fatalf("promptNotes: %v", err)
	}
	if kind != verdictRecorded {
		t.Fatalf("kind = %v, want verdictRecorded", kind)
	}
	if note != "just doesn't feel real somehow" {
		t.Errorf("note = %q, not stored verbatim", note)
	}
	if len(tags) != 0 {
		t.Errorf("tags = %v, want empty (free text has no recognised vocabulary)", tags)
	}
}

// --- runNotesPlan / runNotesPrompt: end-to-end wiring -------------------

func TestRunNotesPlan_RendersSelectedFamily(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(wavref.EnvVar, tmp)
	stageGeneratedRefs(t, tmp, "bassoon-A2")
	outPath := filepath.Join(tmp, "verdicts.jsonl")
	if err := appendVerdict(outPath, earVerdict{Family: roster.Bassoon, Reference: "bassoon-A2", Options: []instrument.ID{roster.Bassoon}, Verdict: "n"}); err != nil {
		t.Fatal(err)
	}

	var out, diag bytes.Buffer
	only := map[string]bool{"n": true, "p": true}
	if err := runNotesPlan(&out, &diag, "", only, false, outPath, "", ""); err != nil {
		t.Fatalf("runNotesPlan: %v", err)
	}
	if !strings.Contains(out.String(), "BEATMO_AB_FAMILY='bassoon'") {
		t.Errorf("expected the bassoon plan, got:\n%s", out.String())
	}
}

func TestRunNotesPlan_NoneLeftEmitsDoneSentinel(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(wavref.EnvVar, tmp)
	outPath := filepath.Join(tmp, "verdicts.jsonl")
	only := map[string]bool{"n": true, "p": true}
	var out, diag bytes.Buffer
	if err := runNotesPlan(&out, &diag, "", only, false, outPath, "", ""); err != nil {
		t.Fatalf("runNotesPlan: %v", err)
	}
	if !strings.Contains(out.String(), "BEATMO_AB_DONE='1'") {
		t.Errorf("expected the done sentinel with no verdicts recorded at all, got:\n%s", out.String())
	}
}

func TestRunNotesPrompt_RecordsNoteInPlace(t *testing.T) {
	tmp := t.TempDir()
	outPath := filepath.Join(tmp, "verdicts.jsonl")
	if err := appendVerdict(outPath, earVerdict{Family: roster.Bassoon, Reference: "bassoon-A2", Options: []instrument.ID{roster.Bassoon}, Verdict: "n"}); err != nil {
		t.Fatal(err)
	}
	if err := appendVerdict(outPath, earVerdict{Family: roster.Koto, Reference: "koto-A3", Options: []instrument.ID{roster.Koto}, Verdict: "y"}); err != nil {
		t.Fatal(err)
	}

	only := map[string]bool{"n": true, "p": true}
	in := strings.NewReader("nasal, thin in the attack\n")
	var out bytes.Buffer
	kind, err := runNotesPrompt(&out, in, "", only, false, outPath, "", "")
	if err != nil {
		t.Fatalf("runNotesPrompt: %v", err)
	}
	if kind != verdictRecorded {
		t.Fatalf("kind = %v, want verdictRecorded", kind)
	}

	got, err := readVerdicts(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("runNotesPrompt changed row count to %d, want 2", len(got))
	}
	var bassoon earVerdict
	for _, v := range got {
		if v.Family == roster.Bassoon {
			bassoon = v
		}
		if v.Family == roster.Koto && v.Note != "" {
			t.Errorf("koto (verdict=y, not selected) must not be touched, got note %q", v.Note)
		}
	}
	if bassoon.Note != "nasal, thin in the attack" {
		t.Errorf("bassoon.Note = %q, want the recorded note", bassoon.Note)
	}
	if !reflect.DeepEqual(bassoon.Tags, []string{"nasal", "thin", "attack"}) {
		t.Errorf("bassoon.Tags = %v, want [nasal thin attack] (\"attack\" appears in \"in the attack\")", bassoon.Tags)
	}
	if bassoon.Verdict != "n" {
		t.Errorf("bassoon.Verdict changed to %q, want unchanged n", bassoon.Verdict)
	}
}

func TestRunNotesPrompt_SkipDoesNotRecord(t *testing.T) {
	tmp := t.TempDir()
	outPath := filepath.Join(tmp, "verdicts.jsonl")
	if err := appendVerdict(outPath, earVerdict{Family: roster.Bassoon, Verdict: "n"}); err != nil {
		t.Fatal(err)
	}
	only := map[string]bool{"n": true, "p": true}
	kind, err := runNotesPrompt(&bytes.Buffer{}, strings.NewReader("s\n"), "", only, false, outPath, "", "")
	if err != nil {
		t.Fatalf("runNotesPrompt: %v", err)
	}
	if kind != verdictSkipped {
		t.Fatalf("kind = %v, want verdictSkipped", kind)
	}
	got, err := readVerdicts(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Note != "" {
		t.Errorf("skip must not write a note, got %q", got[0].Note)
	}
}

func TestRunNotesPrompt_QuitReturnsQuitKind(t *testing.T) {
	tmp := t.TempDir()
	outPath := filepath.Join(tmp, "verdicts.jsonl")
	if err := appendVerdict(outPath, earVerdict{Family: roster.Bassoon, Verdict: "n"}); err != nil {
		t.Fatal(err)
	}
	only := map[string]bool{"n": true, "p": true}
	kind, err := runNotesPrompt(&bytes.Buffer{}, strings.NewReader("q\n"), "", only, false, outPath, "", "")
	if err != nil {
		t.Fatalf("runNotesPrompt: %v", err)
	}
	if kind != verdictQuit {
		t.Errorf("kind = %v, want verdictQuit", kind)
	}
}

// --- cmdAudition -notes: CLI wiring -------------------------------------

func TestCmdAudition_NotesFlagRecordsNote(t *testing.T) {
	tmp := t.TempDir()
	outPath := filepath.Join(tmp, "v.jsonl")
	if err := appendVerdict(outPath, earVerdict{Family: roster.Bassoon, Verdict: "n"}); err != nil {
		t.Fatal(err)
	}
	restoreStdin := swapStdin(t, "nasal\n")
	defer restoreStdin()

	err := cmdAudition([]string{"-notes", "-instrument", string(roster.Bassoon), "-out", outPath})
	if err != nil {
		t.Fatalf("cmdAudition -notes: %v", err)
	}
	got, err := readVerdicts(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Note != "nasal" {
		t.Errorf("bassoon.Note = %q, want nasal", got[0].Note)
	}
}

func TestCmdAudition_NotesRejectsBadOnly(t *testing.T) {
	tmp := t.TempDir()
	outPath := filepath.Join(tmp, "v.jsonl")
	err := cmdAudition([]string{"-notes", "-only", "bogus", "-out", outPath})
	if err == nil {
		t.Error("cmdAudition -notes -only bogus should error")
	}
}

func TestCmdAudition_NotesQuitReturnsSentinel(t *testing.T) {
	tmp := t.TempDir()
	outPath := filepath.Join(tmp, "v.jsonl")
	if err := appendVerdict(outPath, earVerdict{Family: roster.Bassoon, Verdict: "n"}); err != nil {
		t.Fatal(err)
	}
	restoreStdin := swapStdin(t, "q\n")
	defer restoreStdin()
	err := cmdAudition([]string{"-notes", "-instrument", string(roster.Bassoon), "-out", outPath})
	if !errors.Is(err, errAuditionQuit) {
		t.Errorf("cmdAudition -notes quit = %v, want errAuditionQuit", err)
	}
}
