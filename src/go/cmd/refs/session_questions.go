//go:build !js

package main

import (
	"fmt"
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// session_questions.go -- the words a session asks about.
//
// Two rounds share ONE vocabulary, the word salad:
//
//   - QUICK (the default): the listener ticks the words that describe each
//     clip. Same words, same order, for every instrument, so the numbers
//     become muscle memory and a word means the same thing on every family.
//   - DEEP (`refs session -deep`, `make ab-tuned DEEP=1`): on top of that,
//     a 1-5 rating of a few words that matter for THIS instrument --
//     "1 not X enough · 3 about right · 5 too X" -- each a word a
//     descriptor measures, so `refs validate` can check the answer against
//     the measurement.
//
// The listener always reads the WORD; the stored answer is turned to the
// word's axis (sessionScale: +2 always means "the synth reads HIGH on Axis").
// "warm" is the brightness axis read backwards, so "too warm" (5) is stored
// as -2 on brightness.

// sessionWord is one word of the salad. Axis is the descriptor that measures
// it ("" = describe only: no descriptor measures it yet, so it is never a
// deep word); Falls says more of the word reads LOWER on Axis.
type sessionWord struct {
	Word  string
	Group string
	Axis  string
	Falls bool
}

// wordSalad is the shared vocabulary, in menu order. Every word is one the
// listener was offered before (the former global and per-family lists) or
// wrote in a note; every binding is one an earlier question or a `refs
// validate -terms` rule already made (validate_terms.go), cited per line.
var wordSalad = []sessionWord{
	// colour
	{Word: "bright", Group: "colour", Axis: fingerprint.IDBrightnessRatio},                // oboe warmth: "too bright"; guitar note "too pitched up or bright"
	{Word: "dull", Group: "colour", Axis: fingerprint.IDBrightnessRatio, Falls: true},     // bells brightness: "too dull"
	{Word: "warm", Group: "colour", Axis: fingerprint.IDBrightnessRatio, Falls: true},     // oboe warmth: "too warm / dark"
	{Word: "deep", Group: "colour", Axis: fingerprint.IDBrightnessRatio, Falls: true},     // rimshot note "deep, lower in tone"; the map: darker
	{Word: "harsh", Group: "colour"},                                                      //
	{Word: "thin", Group: "colour", Axis: fingerprint.IDSnareLowBody, Falls: true},        // the 80-400 Hz "fat bottom" (a snare-family axis)
	{Word: "full", Group: "colour", Axis: fingerprint.IDSnareLowBody},                     // snare body: "too much body"; rimshot note "full"
	{Word: "muddy", Group: "colour"},                                                      //
	{Word: "hollow", Group: "colour"},                                                     //
	{Word: "nasal", Group: "colour", Axis: fingerprint.IDFormant1},                        // oboe: "too nasal"
	{Word: "reedy", Group: "colour", Axis: fingerprint.IDFormant1},                        // oboe: "not reedy enough"
	{Word: "metallic", Group: "colour", Axis: fingerprint.IDModeCountPerOctave},           // bells metallic complexity: "too many overtones"
	{Word: "plain", Group: "colour", Axis: fingerprint.IDModeCountPerOctave, Falls: true}, // bells: "too plain"
	{Word: "glassy", Group: "colour"},                                                     //
	{Word: "wooden", Group: "colour"},                                                     // the mode-harmonicity rule is an unverified hypothesis
	{Word: "plasticky", Group: "colour"},                                                  //
	// air and noise
	{Word: "airy", Group: "noise", Axis: fingerprint.IDNoisePersistence},            // validate -terms: noise persistence
	{Word: "breathy", Group: "noise", Axis: fingerprint.IDNoisePersistence},         // oboe breathiness: "too breathy"
	{Word: "hissy", Group: "noise", Axis: fingerprint.IDNoiseFlatness, Falls: true}, // snare wire vs hiss: "too hissy" (coloured noise)
	{Word: "rattly", Group: "noise", Axis: fingerprint.IDNoiseFlatness},             // snare wire vs hiss: "too much broadband wire"
	{Word: "buzzy", Group: "noise"},                                                 //
	{Word: "papery", Group: "noise"},                                                //
	// attack
	{Word: "snappy", Group: "attack", Axis: fingerprint.IDOnsetBite},             // snare crack: "too much crack"; rimshot note "cracky (snap)"
	{Word: "crisp", Group: "attack"},                                             //
	{Word: "clanky", Group: "attack", Axis: fingerprint.IDOnsetBite},             // bells strike: "too much clank"
	{Word: "fluty", Group: "attack", Axis: fingerprint.IDOnsetBite, Falls: true}, // validate -terms: a string with no bow bite reads as blown
	// time and movement
	{Word: "resonant", Group: "time", Axis: fingerprint.IDBandT60},                  // bells ring: "rings too long"; harp note "let it ring"
	{Word: "tight", Group: "time", Axis: fingerprint.IDDecayT20, Falls: true},       // snare tail: "too short"
	{Word: "ghostly", Group: "time", Axis: fingerprint.IDDecayT20},                  // validate -terms: "ghost / lingers", the synth outlives the reference
	{Word: "wavy", Group: "time", Axis: fingerprint.IDModeBeatingDepth},             // bells waviness: "too wavy"
	{Word: "alive", Group: "time", Axis: fingerprint.IDModAMFlatness},               // oboe steadiness: "moves unevenly / breathes irregularly"
	{Word: "static", Group: "time", Axis: fingerprint.IDModAMFlatness, Falls: true}, // oboe steadiness: "wobbles like a machine (one fixed rate)"
	// overall
	{Word: "boxy", Group: "overall"},
	{Word: "synthetic", Group: "overall"},
	{Word: "real", Group: "overall"},
}

// saladWords is the word salad as the menu numbers it.
func saladWords() []string {
	out := make([]string, len(wordSalad))
	for i, w := range wordSalad {
		out[i] = w.Word
	}
	return out
}

// wordMenu prints vocab numbered in order, one salad group per line.
func wordMenu(vocab []string) string {
	group := map[string]string{}
	for _, w := range wordSalad {
		group[w.Word] = w.Group
	}
	var b strings.Builder
	last := "\x00"
	for i, w := range vocab {
		if g := group[w]; g != last {
			if g == "" {
				g = "other"
			}
			fmt.Fprintf(&b, "\n  %s:", g)
			last = group[w]
		}
		fmt.Fprintf(&b, "  %d %s", i+1, w)
	}
	return b.String()
}

// familyDeepWords is what the DEEP round rates per instrument: the words
// that matter for it -- from the listener's notes and the match map's gaps
// (tune_picks.json) -- one per axis.
var familyDeepWords = map[instrument.ID][]string{
	roster.Oboe:         {"breathy", "warm", "nasal", "alive"},                // "way too airy/breathy, they need warmness"
	roster.Flute:        {"breathy", "bright", "alive"},                       // "very breathy"; colour gap
	roster.Clarinet:     {"breathy", "warm", "reedy", "alive"},                // colour gap
	roster.Trombone:     {"bright", "breathy", "alive"},                       // darker by a factor of three
	roster.Tuba:         {"bright", "breathy", "alive"},                       // darker by a factor of three
	roster.Cello:        {"breathy", "bright", "fluty", "alive"},              // "still too airy and breathy", "needs more deep string bowing texture"
	roster.Violin:       {"fluty", "bright", "breathy", "alive"},              // "still has this slightly fluty quality"; darker
	roster.Rimshot:      {"snappy", "rattly", "full", "tight"},                // "full, deep, lower in tone and rattly/vibrational and cracky (snap)"
	roster.Snare:        {"rattly", "full", "snappy", "tight"},                // the snare questions: wire, body, crack, tail
	roster.GuitarNylon:  {"bright", "resonant", "snappy"},                     // "too pitched up or bright"
	roster.Harp:         {"resonant", "bright", "metallic"},                   // "let it ring", "brighter ... more metallic"
	roster.TubularBells: {"resonant", "wavy", "metallic", "bright", "clanky"}, // the bells questions: ring, waves, overtones, brightness, strike
}

// genericDeepWords is the deep round of an instrument with no list yet.
var genericDeepWords = []string{"bright", "breathy", "snappy", "resonant"}

// sessionQuestion is one deep word as asked: the word and its axis.
type sessionQuestion struct {
	Key   string // the word
	Text  string // what the listener reads, and what is stored as sessionScale.Q
	Axis  string
	Falls bool
}

func (w sessionWord) question() sessionQuestion {
	return sessionQuestion{Key: w.Word, Text: w.Word, Axis: w.Axis, Falls: w.Falls}
}

// storedAnswer turns a 1-5 answer on the word into the centred answer on the
// axis: +2 always means the synth reads HIGH on Axis.
func (q sessionQuestion) storedAnswer(n int) int {
	if q.Falls {
		return 3 - n
	}
	return n - 3
}

// prompt is the scale as the listener reads it, for one clip.
func (q sessionQuestion) prompt(id string) string {
	return fmt.Sprintf("%s — clip %s: 1 not %s enough · 3 about right · 5 too %s", q.Text, id, q.Text, q.Text)
}

// deepQuestions is the deep round for family: its words that a descriptor
// measures (a word outside the salad, or describe-only, is dropped -- the
// table's test catches that).
func deepQuestions(family instrument.ID) []sessionQuestion {
	words, ok := familyDeepWords[family]
	if !ok {
		words = genericDeepWords
	}
	var out []sessionQuestion
	for _, w := range words {
		for _, sw := range wordSalad {
			if sw.Word == w && sw.Axis != "" {
				out = append(out, sw.question())
			}
		}
	}
	return out
}
