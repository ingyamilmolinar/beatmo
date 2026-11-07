//go:build !js

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// A recording is labelled by the catalogue whichever form it arrives in:
// the catalogue's own converted file, the local file a ref was adopted from,
// or the freesound download whose sound id the ref's preview URL carries.
func TestMatchCatalogueRefRecognisesEveryFormOfARecording(t *testing.T) {
	local, _ := filepath.Abs(filepath.Join(t.TempDir(), "Take 3.wav"))
	c := wavref.Cat{Refs: []wavref.Ref{
		{ID: "cello-freesound-alt", Tier: "instrument", Instruments: []instrument.ID{roster.Cello}, File: "cello_freesound.wav",
			Source: &wavref.Source{Provider: "freesound", URL: "https://cdn.freesound.org/previews/257/257996_43-hq.mp3"}},
		{ID: "kick-local", Tier: "instrument", Instruments: []instrument.ID{roster.KickAiry}, File: "kick-local.wav",
			Source: &wavref.Source{Provider: "local", URL: local}},
		{ID: "sitar-gap", Tier: "instrument", Instruments: []instrument.ID{roster.Sitar}, Status: wavref.StatusNeedsSource},
	}}
	cases := map[string]string{
		"/anywhere/cello_freesound.wav":                  "cello-freesound-alt", // the converted catalogue file
		"/home/u/Downloads/257996__xserra__cello-d2.wav": "cello-freesound-alt", // the raw download
		local: "kick-local", // the adopted local source
		"/home/u/Downloads/999__someone__sitar.wav": "", // unknown sound id
		"/x/2579960__other__thing.wav":              "", // a longer id is a different sound
	}
	for path, want := range cases {
		r, ok := matchCatalogueRef(path, c)
		if got := map[bool]string{true: r.ID, false: ""}[ok]; got != want {
			t.Errorf("%s: labelled %q, want %q", path, got, want)
		}
	}
}

// Silent under their thresholds: a map full of "brighter (1001 vs 1000 Hz)" would bury the real differences.
func TestMapDirection(t *testing.T) {
	ref := fingerprint.EssenceTraits{BrightnessHz: 1000, Noise: 0.1, DecayDB: -3}
	if d := mapDirection(ref, &fingerprint.EssenceTraits{BrightnessHz: 1100, Noise: 0.12, DecayDB: -5}); d != "" {
		t.Errorf("sub-threshold differences reported: %q", d)
	}
	d := mapDirection(ref, &fingerprint.EssenceTraits{BrightnessHz: 500, Noise: 0.4, DecayDB: -20})
	for _, w := range []string{"darker", "noisier", "dies faster"} {
		if !strings.Contains(d, w) {
			t.Errorf("%q lacks %q", d, w)
		}
	}
	d = mapDirection(ref, &fingerprint.EssenceTraits{BrightnessHz: 2000, Noise: 0.0, DecayDB: 0})
	for _, w := range []string{"brighter", "cleaner"} {
		if !strings.Contains(d, w) {
			t.Errorf("%q lacks %q", d, w)
		}
	}
	if d := mapDirection(ref, nil); d != "" {
		t.Errorf("no traits must say nothing, got %q", d)
	}
}

func TestMapWorstTermNamesWhatLostIt(t *testing.T) {
	best := &fingerprint.EssenceScore{Color: 0.2, Evolution: 0.1, Texture: 0.05, Decay: 0.1}
	cand := &fingerprint.EssenceScore{Color: 0.3, Evolution: 0.6, Texture: 0.05, Decay: 0.1}
	if got := mapWorstTerm(cand, best); got != "evolution +0.50" {
		t.Errorf("%q, want evolution +0.50", got)
	}
	// Closer on every term: nothing lost it.
	if got := mapWorstTerm(best, cand); got != "" {
		t.Errorf("%q, want nothing", got)
	}
}

// Coverage states: a recording of it ranking it top 3 = matched; only lower
// = fidelity gap; none of its own but top 3 elsewhere = stand-in; else
// uncovered.
func TestBuildInstrumentMapStates(t *testing.T) {
	rank := func(ids ...instrument.ID) []matchCandidate {
		var out []matchCandidate
		for _, id := range ids {
			out = append(out, matchCandidate{Instrument: id})
		}
		return out
	}
	recs := []mapRecording{
		{e: matchEntry{Name: "cello.wav", Ranking: rank(roster.Viola, roster.Cello, roster.Sax, roster.Harp, roster.Oboe)}, label: []instrument.ID{roster.Cello}},
		{e: matchEntry{Name: "oboe.wav", Ranking: rank(roster.Sax, roster.Clarinet, roster.Viola, roster.Harp, roster.Oboe)}, label: []instrument.ID{roster.Oboe}},
		{e: matchEntry{Name: "mystery.wav", Ranking: rank(roster.Harp, roster.Sitar, roster.Viola, roster.Oboe, roster.Cello)}},
	}
	got := map[string]string{}
	for _, mi := range buildInstrumentMap(recs, wavref.Cat{}, []instrument.ID{roster.Cello, roster.Oboe, roster.Sitar, roster.Tuba}) {
		got[string(mi.id)] = mi.state
	}
	want := map[instrument.ID]string{roster.Cello: mapMatched, roster.Oboe: mapFidelityGap, roster.Sitar: mapStandIn, roster.Tuba: mapUncovered}
	for id, w := range want {
		if got[string(id)] != w {
			t.Errorf("%s: %q, want %q", id, got[string(id)], w)
		}
	}
}

func TestExpandMatchGlobTakesSeveralGlobs(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	for _, p := range []string{filepath.Join(a, "x.wav"), filepath.Join(a, "y.txt"), filepath.Join(b, "z.wav")} {
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := expandMatchGlob(filepath.Join(a, "*.wav") + ", " + filepath.Join(b, "*.wav") + "," + filepath.Join(a, "x.wav"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || filepath.Base(got[0]) != "x.wav" && filepath.Base(got[1]) != "x.wav" {
		t.Fatalf("got %v, want x.wav and z.wav once each", got)
	}
}
