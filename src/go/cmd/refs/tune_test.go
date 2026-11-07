//go:build !js

package main

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/clone"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

func tuneDefs(inst instrument.ID) map[string]audio.ParamDef {
	defs := map[string]audio.ParamDef{}
	for _, d := range audio.InstrumentParamDefs(inst) {
		defs[d.Name] = d
	}
	return defs
}

func shippedDoc(t *testing.T, inst instrument.ID) instrument.Doc {
	t.Helper()
	d, ok := audio.ShippedDoc(inst)
	if !ok {
		t.Fatalf("%s is not a shipped instrument", inst)
	}
	return d
}

// TestTunePicksAreValid: every pick is a roster instrument, says why it was
// picked, and its restriction, edits and harmonics resolve to a search.
func TestTunePicksAreValid(t *testing.T) {
	picks, err := loadTunePicks()
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) == 0 {
		t.Fatal("no picks")
	}
	cat, err := loadCatalogue()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, p := range picks {
		if !slices.Contains(rosterIDs(), p.Inst) {
			t.Errorf("%s: not a roster instrument", p.Inst)
			continue
		}
		if seen[string(p.Inst)] {
			t.Errorf("%s: picked twice", p.Inst)
		}
		seen[string(p.Inst)] = true
		if strings.TrimSpace(p.Why) == "" {
			t.Errorf("%s: a pick must say why (the listener's note and the map's gap)", p.Inst)
		}
		start, defs, err := tuneStart(p)
		if err != nil {
			t.Errorf("%s: %v", p.Inst, err)
			continue
		}
		_, dims, err := tuneSpace(start, defs, p)
		if err != nil {
			t.Errorf("%s: %v", p.Inst, err)
			continue
		}
		if len(dims) == 0 {
			t.Errorf("%s: nothing to search", p.Inst)
		}
		for _, f := range p.Only {
			if !slices.ContainsFunc(dims, func(d tuneDim) bool { return d.Field == f }) {
				t.Errorf("%s: only-field %s is not a searchable value of the document", p.Inst, f)
			}
		}
		for _, id := range p.Skip {
			if !slices.ContainsFunc(cat.ForInstrument(p.Inst), func(r wavref.Ref) bool { return r.ID == id }) {
				t.Errorf("%s: skip %s is not a catalogued recording of it", p.Inst, id)
			}
		}
	}
}

// The origin IS the shipped voice bit for bit, so a run that finds nothing better proposes nothing.
func TestTuneStartsAtTheShippedDocument(t *testing.T) {
	reg := audio.BuiltinModuleRegistry()
	picks, _ := loadTunePicks()
	for _, p := range picks {
		ship, defs, err := tuneStart(p) // the donor's document for a pick From another instrument
		if err != nil {
			t.Fatal(err)
		}
		d0, dims, err := tuneSpace(ship, defs, p)
		if err != nil {
			t.Fatal(err)
		}
		at0, err := applyTune(d0, dims, make([]float64, len(dims)))
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(instrument.Flatten(at0, reg), instrument.Flatten(d0, reg)) {
			t.Errorf("%s: the origin of the search does not reproduce its start", p.Inst)
		}
		if len(p.Set) == 0 && p.AddHarmonics == 0 && instrument.ContentHash(d0) != instrument.ContentHash(ship) {
			t.Errorf("%s: a pick with no edits must start from the shipped document", p.Inst)
		}
		for _, dm := range dims {
			if dm.Start < dm.Lo || dm.Start > dm.Hi {
				t.Errorf("%s %s: start %g outside its box [%g, %g]", p.Inst, dm.Field, dm.Start, dm.Lo, dm.Hi)
			}
			name := dm.Field
			if _, rest, ok := strings.Cut(name, "_"); ok && strings.HasPrefix(name, "gen") {
				name = rest
			}
			if tuneSkippedValues[name] {
				t.Errorf("%s: structural value %s is searched", p.Inst, dm.Field)
			}
		}
	}
}

// A pick From a donor moves the donor's values while the candidate stays the picked instrument (id and name), so
// tune-adopt replaces that instrument and nothing else.
func TestTuneFromStartsAtTheDonorDocument(t *testing.T) {
	reg := audio.BuiltinModuleRegistry()
	p := tunePick{Inst: roster.Rimshot, From: roster.SnareFat, Why: "test"}
	d0, defs, err := tuneStart(p)
	if err != nil {
		t.Fatal(err)
	}
	ship, donor := shippedDoc(t, roster.Rimshot), shippedDoc(t, roster.SnareFat)
	if d0.ID != ship.ID || d0.Name != ship.Name {
		t.Fatalf("the candidate must stay the rimshot: id %q name %q", d0.ID, d0.Name)
	}
	if !slices.Equal(instrument.Flatten(d0, reg), instrument.Flatten(donor, reg)) {
		t.Fatal("the search must start from the donor's sound, bit for bit")
	}
	if !strings.Contains(d0.Notes, "snare-fat") {
		t.Errorf("the document must say where it came from: %q", d0.Notes)
	}
	_, dims, err := tuneSpace(d0, defs, p)
	if err != nil {
		t.Fatal(err)
	}
	donorVals := audio.FlatFieldMap(donor)
	for _, dm := range dims {
		if v, ok := donorVals[dm.Field]; !ok || v != dm.Start {
			t.Errorf("%s starts at %g, the donor holds %g", dm.Field, dm.Start, v)
		}
	}
	if len(dims) == 0 {
		t.Fatal("nothing to search")
	}
	if _, _, err := tuneStart(tunePick{Inst: roster.Rimshot, From: "no-such-voice"}); err == nil {
		t.Fatal("an unknown donor must be an error")
	}
	// Without From the start is the shipped document itself.
	if d, _, err := tuneStart(tunePick{Inst: roster.Rimshot}); err != nil || instrument.ContentHash(d) != instrument.ContentHash(ship) {
		t.Fatalf("no From: %v", err)
	}
}

// TestTuneDimStaysInItsBox: every coordinate maps inside the trust box, the
// origin is the start exactly, and log dims move by ratio.
func TestTuneDimStaysInItsBox(t *testing.T) {
	cut, ok := newTuneDim("filter_cutoff", audio.ParamDef{Min: 20, Max: 20000}, 2200, false)
	if !ok || !cut.Log || math.Abs(cut.Lo-2200/tuneLogReach) > 1e-9 || math.Abs(cut.Hi-2200*tuneLogReach) > 1e-9 {
		t.Fatalf("log dim %+v", cut)
	}
	gain, ok := newTuneDim("gen2_gain", audio.ParamDef{Min: 0, Max: 1.5}, 0.42, false)
	if !ok || gain.Log {
		t.Fatalf("linear dim %+v", gain)
	}
	added, ok := newTuneDim("gen9_gain", audio.ParamDef{Min: 0, Max: 1.5}, 0, true)
	if !ok || added.Lo != 0 || added.Hi != tuneAddedGainMax {
		t.Fatalf("added dim %+v", added)
	}
	for _, d := range []tuneDim{cut, gain, added} {
		if d.value(0) != d.Start {
			t.Errorf("%s: origin %g != start %g", d.Field, d.value(0), d.Start)
		}
		for y := -7.0; y <= 7; y += 0.37 {
			if v := d.value(y); v < d.Lo-1e-12 || v > d.Hi+1e-12 {
				t.Errorf("%s: y=%g -> %g outside [%g, %g]", d.Field, y, v, d.Lo, d.Hi)
			}
		}
	}
	if _, ok := newTuneDim("x", audio.ParamDef{Min: 1, Max: 1}, 1, false); ok {
		t.Error("an empty range must not become a dimension")
	}
}

// TestAddedHarmonicsStartSilent: raising partials from silence extends the
// harmonic series inside the gen bank's slot capacity and, at gain 0, leaves
// every existing slot exactly as shipped.
func TestAddedHarmonicsStartSilent(t *testing.T) {
	reg := audio.BuiltinModuleRegistry()
	ship := shippedDoc(t, roster.Trombone)
	// Its five partials and breath, so the bank has room whatever later tuning raised above them.
	ship.Layers = ship.Layers[:6]
	d, added, err := addHarmonicLayers(ship, reg, 6)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Layers) > reg.Slots() || len(added) == 0 {
		t.Fatalf("%d layers (cap %d), %d added", len(d.Layers), reg.Slots(), len(added))
	}
	if len(ship.Layers) == len(d.Layers) || len(ship.Layers) != len(d.Layers)-len(added) {
		t.Fatalf("shipped %d layers, now %d, added %d", len(ship.Layers), len(d.Layers), len(added))
	}
	prev := 0.0
	for _, l := range d.Layers[:len(ship.Layers)] {
		if l.Type == "gen.wavetable" {
			prev = math.Max(prev, l.Values["freq"])
		}
	}
	for _, l := range d.Layers[len(ship.Layers):] {
		if l.Values["gain"] != 0 || l.Values["freq"] != prev+1 {
			t.Fatalf("added partial %v after H%g", l.Values, prev)
		}
		prev++
	}
	a, b := instrument.FieldValues(ship, reg), instrument.FieldValues(d, reg)
	for f, v := range a {
		if b[f] != v && !strings.HasPrefix(f, "gen") {
			t.Errorf("%s moved %g -> %g", f, v, b[f])
		}
	}
	if _, _, err := addHarmonicLayers(shippedDoc(t, roster.Harp), reg, 2); err == nil {
		t.Error("a document with no partial series must refuse extra harmonics")
	}
}

// TestTuneSearchDescendsAndIsSeeded: on an injected landscape the search
// beats its start, and the same seed gives the same answer.
func TestTuneSearchDescendsAndIsSeeded(t *testing.T) {
	p := tunePick{Inst: roster.Oboe}
	d0, dims, err := tuneSpace(shippedDoc(t, roster.Oboe), tuneDefs(roster.Oboe), p)
	if err != nil {
		t.Fatal(err)
	}
	reg := audio.BuiltinModuleRegistry()
	// The "recording" wants a darker filter and less noise than the oboe.
	eval := func(d instrument.Doc, _ tuneRec) (float64, error) {
		v := instrument.FieldValues(d, reg)
		return math.Abs(math.Log(v["filter_cutoff"]/3000)) + 4*math.Abs(v["gen1_gain"]-0.1), nil
	}
	o := tuneOpts{Gens: 25, Workers: 4, Stall: 25, Seed: 7, Sigma: 0.35}
	recs := []tuneRec{{Name: "fake"}}
	a, err := tuneSearch(d0, dims, recs, eval, o, nil)
	if err != nil {
		t.Fatal(err)
	}
	if a.Y == nil || a.Best >= a.Start*0.8 {
		t.Fatalf("no descent: start %.4f best %.4f", a.Start, a.Best)
	}
	b, _ := tuneSearch(d0, dims, recs, eval, o, nil)
	if !slices.Equal(a.Y, b.Y) || a.Best != b.Best {
		t.Fatal("same seed, different answer")
	}
	got, _ := applyTune(d0, dims, a.Y)
	v := instrument.FieldValues(got, reg)
	if v["filter_cutoff"] >= 7000 || v["gen1_gain"] >= 0.37 {
		t.Fatalf("moved the wrong way: cutoff %g noise %g", v["filter_cutoff"], v["gen1_gain"])
	}
	if ch := tuneChanges(dims, a.Y); len(ch) == 0 {
		t.Fatal("no changes reported")
	}
	if z := tuneChanges(dims, nil); len(z) != 0 {
		t.Fatalf("the start reports changes: %v", z)
	}
}

// TestSessionTunedAuditionsTheTunedCandidateAlone: `refs session -tuned`
// plays reference, current and candidate T only, at the current voice's
// note, and records the ship verdict against the candidate's doc_sha.
func TestSessionTunedAuditionsTheTunedCandidateAlone(t *testing.T) {
	t.Setenv(wavref.EnvVar, t.TempDir())
	root := t.TempDir()
	d := shippedDoc(t, roster.Oboe)
	d.Stages["filter"].Values["filter_cutoff"] = 3000
	dir := filepath.Join(root, "oboe")
	os.MkdirAll(dir, 0o755)
	b, _ := instrument.MarshalFile(d)
	os.WriteFile(filepath.Join(dir, "candidate.json"), b, 0o644)
	m, _ := json.Marshal(tuneMeta{Inst: roster.Oboe, Ref: "oboe-A4", SamePitch: true, Why: "too breathy"})
	os.WriteFile(filepath.Join(dir, "meta.json"), m, 0o644)

	fc, err := withDocCandidate(familyCandidates{}, false, roster.Oboe, root, tunedCandidates)
	if err != nil {
		t.Fatal(err)
	}
	if fc.Reference != "oboe-A4" || fc.Base != roster.Oboe || len(fc.Candidates) != 1 {
		t.Fatalf("family %+v", fc)
	}
	c := fc.Candidates[0]
	if c.ID != "t" || !c.SamePitch || c.DocSHA != clone.DocSHA(d) || !strings.Contains(c.Hypothesis, "too breathy") {
		t.Fatalf("candidate T %+v", c)
	}
	if got := cloneFamilies(root); !slices.Equal(got, []instrument.ID{roster.Oboe}) {
		t.Fatalf("tuned families %v", got)
	}

	ref := wavref.Ref{ID: "oboe-A4", Measured: &wavref.Measured{SemisFromA3: 12, DurationSec: 1}}
	render := func(familyCandidates, wavref.Ref) (sessionClips, sessionRenderMeta, error) {
		return sessionClips{refPath: "/R.wav", curPath: "/C.wav", candPaths: map[string]string{"T": "/T.wav"}, order: []string{"R", "C", "T"}},
			sessionRenderMeta{Pitch: 12, Dur: 1}, nil
	}
	out := filepath.Join(t.TempDir(), "ear_sessions.jsonl")
	// rating, vs current, ship gate, the oboe's four deep words, the word salad, ship, note
	in := strings.Join([]string{"", "4", "+", "y", "3", "3", "-", "3", "", "1", ""}, "\n") + "\n"
	var log bytes.Buffer
	k, err := runSessionFamily(&log, strings.NewReader(in), &fakePlayer{}, roster.Oboe, fc, ref, render, out, false, true)
	if err != nil || k != verdictRecorded {
		t.Fatalf("%v %v\n%s", k, err, log.String())
	}
	s := readOneSession(t, out)
	if got := s.Candidates[0]; got.ID != "T" || got.DocSHA != clone.DocSHA(d) || got.Accept != "y" || s.Ship != "T" {
		t.Fatalf("tuned candidate %+v ship %q", got, s.Ship)
	}
	if got := s.Candidates[0]; s.Mode != sessionModeDeep || len(got.Scales) != 3 || len(got.Adjectives) != 0 {
		t.Fatalf("deep oboe: mode %q, %d scales (four words, one skipped), words %v", s.Mode, len(got.Scales), got.Adjectives)
	}
	if !strings.Contains(log.String(), "Ship candidate T") {
		t.Fatalf("no ship prompt:\n%s", log.String())
	}
}

// stageTunedFixture writes a tuned candidate for inst (its shipped document
// with the cutoff moved) against refID under root, and returns its doc_sha.
func stageTunedFixture(t *testing.T, root, inst, refID instrument.ID) string {
	t.Helper()
	d := shippedDoc(t, inst)
	d.Stages["filter"].Values["filter_cutoff"] = 3000
	dir := filepath.Join(string(root), string(inst))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	b, _ := instrument.MarshalFile(d)
	m, _ := json.Marshal(tuneMeta{Inst: inst, Ref: string(refID), SamePitch: true, Why: "fixture"})
	if os.WriteFile(filepath.Join(dir, "candidate.json"), b, 0o644) != nil || os.WriteFile(filepath.Join(dir, "meta.json"), m, 0o644) != nil {
		t.Fatal("staging the fixture")
	}
	return clone.DocSHA(d)
}

// Reviewed = a record for EXACTLY the candidate on disk (a re-tuned one is pending again); a deep round counts only deep
// records; a named family is always asked; resume=false asks all. Nothing recorded is rewritten.
func TestAbTunedResumesAtTheFirstUnreviewedFamily(t *testing.T) {
	t.Setenv(wavref.EnvVar, t.TempDir()) // no reference audio: reaching a family's render fails, naming it
	root := t.TempDir()
	// Name order: the reviewed clarinet comes first.
	clarSHA := stageTunedFixture(t, instrument.ID(root), roster.Clarinet, "clarinet-A4")
	stageTunedFixture(t, instrument.ID(root), roster.Oboe, "oboe-A4")
	out := filepath.Join(t.TempDir(), "ear_sessions.jsonl")
	rec := func(fam, sha, mode string) earSession {
		return earSession{Session: "2026-10-21T00:00:00Z", Family: instrument.ID(fam), Mode: mode, Reference: fam + "-A4", Note: fam + " note",
			Candidates: []sessionCandidate{{sessionClip: sessionClip{ID: "T", Base: instrument.ID(fam), RecipeSHA: sha, DocSHA: sha}, Rating: 4}}}
	}
	// oboe was reviewed too, but for a candidate that has since been re-tuned.
	for _, s := range []earSession{rec(string(roster.Clarinet), clarSHA, ""), rec(string(roster.Oboe), "sha256:retunedsince0000", sessionModeQuick)} {
		if err := appendSession(out, s); err != nil {
			t.Fatal(err)
		}
	}
	before, _ := os.ReadFile(out)
	candPath, err := defaultCandidatesPath()
	if err != nil {
		t.Fatal(err)
	}
	run := func(families []instrument.ID, deep, resume bool) (string, error) {
		var w strings.Builder
		err := runSession(&w, strings.NewReader(""), &fakePlayer{}, families, candPath, out, false, deep, resume, "", root)
		return w.String(), err
	}

	log, err := run(nil, false, true)
	if err == nil || !strings.Contains(err.Error(), "oboe-A4") {
		t.Fatalf("the quick round must skip the reviewed clarinet and stop at the re-tuned oboe: err %v\n%s", err, log)
	}
	if !strings.Contains(log, "clarinet (2026-10-21") || !strings.Contains(log, "already reviewed") || strings.Contains(log, "=== clarinet") {
		t.Fatalf("the skip must be reported, and the oboe not played:\n%s", log)
	}
	for _, c := range []struct {
		name     string
		families []instrument.ID
		deep     bool
		resume   bool
	}{
		{"a deep round counts only deep records", nil, true, true},
		{"resume=false asks everything", nil, false, false},
		{"a named family is always asked", []instrument.ID{roster.Clarinet}, false, true},
	} {
		if _, err := run(c.families, c.deep, c.resume); err == nil || !strings.Contains(err.Error(), "clarinet-A4") {
			t.Errorf("%s: want the clarinet asked, got err %v", c.name, err)
		}
	}
	if after, _ := os.ReadFile(out); string(after) != string(before) {
		t.Fatalf("the recorded sessions changed:\n%s\n->\n%s", before, after)
	}
}
