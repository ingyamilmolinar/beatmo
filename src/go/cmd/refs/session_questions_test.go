//go:build !js

package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// TestWordSaladIsOneListForEveryInstrument: the quick round offers the SAME
// words, in the same order, whatever the instrument, so the numbers become
// muscle memory and a word means the same thing on every family. It keeps
// every word the listener has been offered before (the old per-family lists
// and the global fallback) and the words of their own notes.
func TestWordSaladIsOneListForEveryInstrument(t *testing.T) {
	words := saladWords()
	seen := map[string]bool{}
	for _, w := range words {
		if seen[w] {
			t.Errorf("%q twice in the word salad", w)
		}
		seen[w] = true
	}
	for _, w := range []string{
		// the former global list
		"airy", "dull", "harsh", "thin", "full", "metallic", "wooden", "plasticky", "buzzy", "hollow", "nasal", "muddy", "static", "alive", "synthetic", "real",
		// the former bells / snare / oboe lists
		"glassy", "wavy", "plain", "clanky", "hissy", "papery", "boxy", "rattly", "crisp", "tight", "ghostly", "reedy", "warm", "fluty",
		// the listener's notes: "too pitched up or bright", "airy/breathy",
		// "deep ... cracky (snap)", "let it ring"
		"bright", "breathy", "deep", "snappy", "resonant",
	} {
		if !seen[w] {
			t.Errorf("the word salad lost %q", w)
		}
	}
	for _, w := range wordSalad {
		if w.Group == "" {
			t.Errorf("%q has no group to print under", w.Word)
		}
		if w.Axis != "" {
			if _, ok := fingerprint.DefaultRegistry().ByID(w.Axis); !ok {
				t.Errorf("%q binds unknown descriptor id %q", w.Word, w.Axis)
			}
		}
	}
}

// TestDeepWordsAreRelevantAndMeasured: every instrument a session can audition
// (the curated tune picks and the candidates file) has its own short list of
// deep words, each one a word of the salad that a descriptor measures, on
// distinct axes -- two words on one axis ask the same question twice.
func TestDeepWordsAreRelevantAndMeasured(t *testing.T) {
	picks, err := loadTunePicks()
	if err != nil {
		t.Fatal(err)
	}
	fams := []instrument.ID{roster.TubularBells}
	for _, p := range picks {
		fams = append(fams, p.Inst)
	}
	if cp, err := defaultCandidatesPath(); err == nil {
		if all, err := loadCandidates(cp); err == nil {
			for f := range all {
				fams = append(fams, f)
			}
		}
	}
	for _, f := range fams {
		if _, ok := familyDeepWords[f]; !ok {
			t.Errorf("%s has no deep words of its own", f)
		}
	}
	for fam, ws := range familyDeepWords {
		if len(ws) < 3 || len(ws) > 6 {
			t.Errorf("%s: %d deep words; want 3-6", fam, len(ws))
		}
		axes := map[string]string{}
		for _, q := range deepQuestions(fam) {
			if q.Axis == "" {
				t.Errorf("%s/%s: a deep word must be measured (bound to an axis)", fam, q.Key)
			}
			if prev, dup := axes[q.Axis]; dup {
				t.Errorf("%s: %s and %s both ask about %s", fam, prev, q.Key, q.Axis)
			}
			axes[q.Axis] = q.Key
		}
		if len(deepQuestions(fam)) != len(ws) {
			t.Errorf("%s: deep words %v are not all words of the salad", fam, ws)
		}
	}
	if len(deepQuestions("no-such-family")) < 3 {
		t.Error("an instrument without a list still gets the generic deep words")
	}
}

// TestDeepScaleIsStoredOnTheAxis: the listener always reads "1 not X enough
// · 3 about right · 5 too X" -- the word, never the axis -- and the stored
// answer is turned to the axis, so +2 still means "the synth reads HIGH on
// Axis" (the rule `refs validate` scores by). "warm" is the brightness axis
// read backwards: "too warm" is a synth that reads LOW on brightness.
func TestDeepScaleIsStoredOnTheAxis(t *testing.T) {
	q := wordQuestion(t, "warm")
	if q.Axis != fingerprint.IDBrightnessRatio || q.storedAnswer(5) != -2 || q.storedAnswer(1) != 2 || q.storedAnswer(3) != 0 {
		t.Fatalf("warm: axis %s, 5 -> %d, 1 -> %d", q.Axis, q.storedAnswer(5), q.storedAnswer(1))
	}
	if q := wordQuestion(t, "bright"); q.storedAnswer(5) != 2 {
		t.Fatalf("too bright must store +2 on brightness, got %d", q.storedAnswer(5))
	}
	if want := "warm — clip T: 1 not warm enough · 3 about right · 5 too warm"; !strings.HasPrefix(q.prompt("T"), want) {
		t.Fatalf("prompt %q, want it to start %q", q.prompt("T"), want)
	}
	// The polarities the old hand-worded questions pinned, now carried by
	// the words: an irregular, breathing movement reads HIGH on the
	// AM-flatness axis (one fixed LFO rate is one spectral peak, LOW).
	if q := wordQuestion(t, "alive"); q.Axis != fingerprint.IDModAMFlatness || q.storedAnswer(5) != 2 {
		t.Fatalf("alive: %+v", q)
	}
	if q := wordQuestion(t, "static"); q.Axis != fingerprint.IDModAMFlatness || q.storedAnswer(5) != -2 {
		t.Fatalf("static: %+v", q)
	}
	if q := wordQuestion(t, "hissy"); q.Axis != fingerprint.IDNoiseFlatness || q.storedAnswer(5) != -2 {
		t.Fatalf("hissy is coloured noise, LOW flatness: %+v", q)
	}
}

func wordQuestion(t *testing.T, word string) sessionQuestion {
	t.Helper()
	i := slices.IndexFunc(wordSalad, func(w sessionWord) bool { return w.Word == word })
	if i < 0 {
		t.Fatalf("%q is not in the word salad", word)
	}
	return wordSalad[i].question()
}
