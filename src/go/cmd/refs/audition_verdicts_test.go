//go:build !js

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func TestEarVerdict_JSONRoundTrips(t *testing.T) {
	v := earVerdict{
		Family:      roster.Cello,
		Reference:   "cello-A3",
		Options:     []instrument.ID{roster.Cello, roster.CelloWarm},
		Verdict:     "1",
		Chosen:      roster.Cello,
		Note:        "attack too soft, too nasal in the mids",
		SemisFromA3: 0,
	}
	b, err := marshalVerdict(v)
	if err != nil {
		t.Fatalf("marshalVerdict: %v", err)
	}
	got, err := unmarshalVerdict(b)
	if err != nil {
		t.Fatalf("unmarshalVerdict: %v", err)
	}
	if !reflect.DeepEqual(got, v) {
		t.Errorf("round trip mismatch: got %+v, want %+v", got, v)
	}
}

func TestAppendVerdict_DoesNotClobberExistingLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ear_verdicts.jsonl")

	v1 := earVerdict{Family: roster.Cello, Reference: "cello-A3", Options: []instrument.ID{roster.Cello, roster.CelloWarm}, Verdict: "1", Chosen: roster.Cello}
	v2 := earVerdict{Family: roster.Oboe, Reference: "oboe-A4", Options: []instrument.ID{roster.Oboe, roster.OboeFull}, Verdict: "n"}

	if err := appendVerdict(path, v1); err != nil {
		t.Fatalf("appendVerdict 1: %v", err)
	}
	if err := appendVerdict(path, v2); err != nil {
		t.Fatalf("appendVerdict 2: %v", err)
	}

	got, err := readVerdicts(path)
	if err != nil {
		t.Fatalf("readVerdicts: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("readVerdicts returned %d entries, want 2: %+v", len(got), got)
	}
	if got[0].Family != roster.Cello || got[1].Family != roster.Oboe {
		t.Errorf("verdicts out of order or wrong content: %+v", got)
	}
}

func TestReadVerdicts_MissingFileIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist.jsonl")
	got, err := readVerdicts(path)
	if err != nil {
		t.Fatalf("readVerdicts on missing file: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("readVerdicts on missing file = %v, want empty", got)
	}
}

func TestLoadAnsweredFamilies_TracksDistinctFamilies(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ear_verdicts.jsonl")
	if err := appendVerdict(path, earVerdict{Family: roster.Cello, Verdict: "1"}); err != nil {
		t.Fatal(err)
	}
	if err := appendVerdict(path, earVerdict{Family: roster.Cello, Verdict: "r"}); err != nil {
		// r (replay) is never actually persisted by the real flow, but the
		// reader must not choke on whatever IS on disk.
		t.Fatal(err)
	}
	if err := appendVerdict(path, earVerdict{Family: roster.Oboe, Verdict: "n"}); err != nil {
		t.Fatal(err)
	}

	answered, err := loadAnsweredFamilies(path)
	if err != nil {
		t.Fatalf("loadAnsweredFamilies: %v", err)
	}
	if len(answered) != 2 || !answered[roster.Cello] || !answered[roster.Oboe] {
		t.Errorf("answered = %v, want {cello, oboe}", answered)
	}
}

func TestPickFamily_ExplicitInstrumentIgnoresResume(t *testing.T) {
	families := []auditionFamily{
		{Key: roster.Bassoon, Members: []instrument.ID{roster.Bassoon}},
		{Key: roster.Cello, Members: []instrument.ID{roster.Cello, roster.CelloWarm}},
	}
	answered := map[instrument.ID]bool{roster.Cello: true}

	fam, found, err := pickFamily(families, roster.Cello, true, answered, nil)
	if err != nil {
		t.Fatalf("pickFamily: %v", err)
	}
	if !found || fam.Key != roster.Cello {
		t.Errorf("pickFamily(explicit cello) = (%+v, %v), want cello/true even though already answered", fam, found)
	}
}

func TestPickFamily_UnknownExplicitInstrumentErrors(t *testing.T) {
	families := []auditionFamily{{Key: roster.Bassoon, Members: []instrument.ID{roster.Bassoon}}}
	if _, _, err := pickFamily(families, "no-such-family", true, nil, nil); err == nil {
		t.Error("pickFamily accepted an unknown -instrument")
	}
}

func TestPickFamily_AutoPickSkipsAnsweredWhenResuming(t *testing.T) {
	families := []auditionFamily{
		{Key: roster.Bassoon, Members: []instrument.ID{roster.Bassoon}},
		{Key: roster.Cello, Members: []instrument.ID{roster.Cello, roster.CelloWarm}},
		{Key: roster.Oboe, Members: []instrument.ID{roster.Oboe, roster.OboeFull}},
	}
	answered := map[instrument.ID]bool{roster.Bassoon: true}

	fam, found, err := pickFamily(families, "", true, answered, nil)
	if err != nil {
		t.Fatalf("pickFamily: %v", err)
	}
	if !found || fam.Key != roster.Cello {
		t.Errorf("pickFamily(auto, resume) = (%+v, %v), want cello (first unanswered)", fam, found)
	}
}

func TestPickFamily_AutoPickIgnoresAnsweredWhenNotResuming(t *testing.T) {
	families := []auditionFamily{
		{Key: roster.Bassoon, Members: []instrument.ID{roster.Bassoon}},
		{Key: roster.Cello, Members: []instrument.ID{roster.Cello, roster.CelloWarm}},
	}
	answered := map[instrument.ID]bool{roster.Bassoon: true}

	fam, found, err := pickFamily(families, "", false, answered, nil)
	if err != nil {
		t.Fatalf("pickFamily: %v", err)
	}
	if !found || fam.Key != roster.Bassoon {
		t.Errorf("pickFamily(auto, no resume) = (%+v, %v), want bassoon (first overall)", fam, found)
	}
}

func TestPickFamily_AutoPickNoneLeftReportsNotFound(t *testing.T) {
	families := []auditionFamily{{Key: roster.Bassoon, Members: []instrument.ID{roster.Bassoon}}}
	answered := map[instrument.ID]bool{roster.Bassoon: true}

	_, found, err := pickFamily(families, "", true, answered, nil)
	if err != nil {
		t.Fatalf("pickFamily: %v", err)
	}
	if found {
		t.Error("pickFamily(auto, resume) with everything answered should report not-found")
	}
}

// sanity: defaultVerdictsPath resolves to a real, module-root-relative
// path so the CLI's default -out works from any cwd, mirroring how
// wavref.ModuleRoot-based paths behave elsewhere in cmd/refs.
func TestDefaultVerdictsPath_UnderTestdata(t *testing.T) {
	p, err := defaultVerdictsPath()
	if err != nil {
		t.Fatalf("defaultVerdictsPath: %v", err)
	}
	if filepath.Base(p) != "ear_verdicts.jsonl" {
		t.Errorf("defaultVerdictsPath = %q, want basename ear_verdicts.jsonl", p)
	}
	if _, err := os.Stat(filepath.Dir(p)); err != nil {
		t.Errorf("defaultVerdictsPath dir does not exist: %v", err)
	}
}

func TestPickFamily_SessionSkipExcludesEvenWhenNotAnswered(t *testing.T) {
	families := []auditionFamily{
		{Key: roster.Bassoon, Members: []instrument.ID{roster.Bassoon}},
		{Key: roster.Cello, Members: []instrument.ID{roster.Cello, roster.CelloWarm}},
	}
	// bassoon has NO recorded verdict, but is in this session's skip set --
	// unlike resume (persisted across runs), a session skip must exclude it
	// regardless of resume's value.
	fam, found, err := pickFamily(families, "", true, nil, map[instrument.ID]bool{roster.Bassoon: true})
	if err != nil {
		t.Fatalf("pickFamily: %v", err)
	}
	if !found || fam.Key != roster.Cello {
		t.Errorf("pickFamily(sessionSkip=bassoon) = (%+v, %v), want cello", fam, found)
	}
}

func TestPickFamily_ExplicitInstrumentIgnoresSessionSkip(t *testing.T) {
	families := []auditionFamily{{Key: roster.Bassoon, Members: []instrument.ID{roster.Bassoon}}}
	fam, found, err := pickFamily(families, roster.Bassoon, true, nil, map[instrument.ID]bool{roster.Bassoon: true})
	if err != nil {
		t.Fatalf("pickFamily: %v", err)
	}
	if !found || fam.Key != roster.Bassoon {
		t.Errorf("explicit -instrument must override sessionSkip, got (%+v, %v)", fam, found)
	}
}

func TestParseSkipSet_CommaSeparated(t *testing.T) {
	got := parseSkipSet("bassoon,cello")
	if len(got) != 2 || !got[roster.Bassoon] || !got[roster.Cello] {
		t.Errorf("parseSkipSet(\"bassoon,cello\") = %v, want {bassoon,cello}", got)
	}
}

func TestParseSkipSet_EmptyAndStrayCommasIgnored(t *testing.T) {
	got := parseSkipSet("")
	if len(got) != 0 {
		t.Errorf("parseSkipSet(\"\") = %v, want empty", got)
	}
	got = parseSkipSet(",bassoon,,cello,")
	if len(got) != 2 || !got[roster.Bassoon] || !got[roster.Cello] {
		t.Errorf("parseSkipSet with stray commas = %v, want {bassoon,cello}", got)
	}
}
