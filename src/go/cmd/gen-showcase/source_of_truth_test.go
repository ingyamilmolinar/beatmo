package main

import "testing"

// This file validates every template circuit against the researched musical
// "source of truth" — the canonical key/mode of the piece (its diatonic
// pitch-class set, A3=0) and, where the tempo is well-established, its BPM.
// It asserts at the spec level (showcases()), the source build() renders.
//
// Pitch classes use A3=0: A=0,A#=1,B=2,C=3,C#=4,D=5,D#=6,E=7,F=8,F#=9,G=10,G#=11.
// Percussion rows carry no musical pitch and are skipped (see pitchedInstrument).

// pitchedInstrument reports whether an instrument id carries musical pitch.
// The melodic/harmonic instruments are exactly main.go's seededModularInstruments
// set minus conga (which is percussion). Everything else (kick/snare/hihat/ride/
// clave/shaker/sidestick/clap/fm-*) is percussion or a non-diatonic timbre.
func pitchedInstrument(id string) bool {
	switch id {
	case "conga", "conga-open", "conga-tumba", "fm-bell", "fm-lead", "fm-epiano":
		// fm-* are dominant/extended timbres voiced as colour, not strict-scale
		// melodic lines; conga is percussion. Excluded from the diatonic check.
		return false
	}
	return seededModularInstruments[id]
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// pcSet builds a pitch-class set (A3=0) from a list of class ints.
func pcSet(classes ...int) map[int]bool {
	m := make(map[int]bool, len(classes))
	for _, c := range classes {
		m[c] = true
	}
	return m
}

type musicTruth struct {
	stem   string
	bpm    int          // canonical BPM; 0 => tempo is interpretive, skip the check
	bpmTol int          // allowed deviation from bpm
	scale  map[int]bool // allowed diatonic pitch classes (A3=0); nil => skip key check
	note   string       // provenance / why
}

// musicTruths encodes the researched key/tempo for each template. Already-faithful
// templates are documented here too, so the test guards them against regression.
var musicTruths = []musicTruth{
	// ── Public-domain classical (fixed keys; tempo often interpretive) ──
	{stem: "bach-toccata", scale: pcSet(0, 1, 3, 4, 5, 7, 8, 10), note: "D minor (nat+harmonic), BWV 565"},
	{stem: "bach-prelude-c", scale: pcSet(0, 2, 3, 5, 7, 8, 10), note: "C major, BWV 846"},
	{stem: "bach-flute-allemande", scale: pcSet(0, 2, 3, 5, 7, 8, 10, 11), note: "A minor (nat+harmonic), BWV 1013"},
	{stem: "bach-cello-prelude", scale: pcSet(0, 2, 3, 5, 7, 9, 10), note: "G major, BWV 1007"},
	{stem: "felt-prelude", scale: pcSet(0, 2, 3, 5, 7, 8, 10), note: "C major, BWV 846 (felt)"},
	{stem: "vivaldi-spring", scale: pcSet(0, 2, 4, 6, 7, 9, 11), note: "E major, RV 269"},
	{stem: "mozart-k545", scale: pcSet(0, 2, 3, 5, 7, 8, 10), note: "C major, K545"},
	{stem: "pachelbel-violin", scale: pcSet(0, 2, 4, 5, 7, 9, 10), note: "D major, Canon"},
	{stem: "marcello-oboe", scale: pcSet(0, 1, 3, 4, 5, 7, 8, 10), note: "D minor (nat+harmonic) — B-natural is out of key"},
	{stem: "handel-water-horn", scale: pcSet(0, 2, 4, 5, 7, 9, 10), note: "D major, HWV 349"},
	{stem: "cielito-trumpet", bpm: 150, bpmTol: 18, scale: pcSet(0, 2, 4, 5, 7, 9, 10), note: "D major (canonical mariachi-trumpet key); lively mariachi waltz (trimmed from 165)"},

	// ── Jazz / blues ──
	{stem: "jobim-ipanema", scale: pcSet(0, 1, 2, 3, 5, 7, 8, 10), note: "F major (+B from G7)"},
	{stem: "miles-so-what", note: "bimodal D dorian / Eb dorian — key check skipped"},
	{stem: "bbking-thrill-is-gone", bpm: 90, bpmTol: 4, scale: pcSet(0, 1, 2, 4, 5, 7, 8, 9, 10), note: "B minor blues (~88-90 BPM, not 98)"},
	{stem: "sax-blues", scale: pcSet(0, 3, 5, 6, 7, 10), note: "A minor blues scale"},

	// ── Funk / soul / disco ──
	{stem: "wonder-superstition", scale: pcSet(1, 2, 4, 6, 8, 9, 11), note: "Eb minor"},
	{stem: "chic-good-times", scale: pcSet(0, 2, 3, 4, 5, 7, 8, 10, 11), note: "A minor + chromatic C#/G# walk (real Bernard Edwards line, bitmidi 23412)"},
	{stem: "marley-exodus", scale: pcSet(0, 2, 3, 5, 7, 8, 10, 11), note: "A minor + chromatic G# walk-up (real bassline)"},
	{stem: "dre-g-thang", scale: pcSet(0, 2, 4, 5, 7, 9, 10), note: "B natural minor (transcribed from the real MIDI, bitmidi 41193)"},
	{stem: "clav-funk", scale: pcSet(0, 2, 3, 5, 7, 8, 10), note: "A minor (pentatonic-based)"},

	// ── Rock / pop / electronic ──
	{stem: "eagles-hotel-california", scale: pcSet(0, 1, 2, 4, 5, 7, 9, 10, 11), note: "B minor (+F#7/E chromatics)"},
	{stem: "toto-africa", scale: pcSet(0, 1, 2, 4, 6, 7, 9, 11), note: "B major +A (bVII) — see TestAfricaChorusHasFlatSeven"},
	{stem: "blackbox-ride-on-time", bpm: 120, bpmTol: 5, scale: pcSet(0, 2, 3, 5, 7, 8, 10), note: "A minor (Black Box — Ride On Time, real MIDI bitmidi 88846)"},
	{stem: "gaynor-survive", bpm: 117, bpmTol: 6, scale: pcSet(0, 2, 3, 5, 7, 8, 10, 11), note: "A minor (I Will Survive cycle-of-4ths, +G# from E7)"},

	// ── Latin + etudes (groove conventions; in-key) ──
	{stem: "puente-oye-como-va", scale: pcSet(0, 2, 3, 5, 7, 9, 10), note: "A dorian (Oye Como Va, Am7-D9 vamp; real MIDI bitmidi 91454)"},
	{stem: "salsa-vivir", bpm: 105, bpmTol: 8, scale: pcSet(1, 3, 5, 6, 8, 10, 11), note: "C minor (Vivir Mi Vida montuno Cm-Ab-Eb-Bb)"},
	{stem: "bachata-obsesion", bpm: 134, bpmTol: 8, scale: pcSet(0, 2, 4, 6, 7, 9, 11), note: "C# minor (Obsesión, C#m-G#m requinto loop)"},
	{stem: "conga-tumbao", note: "percussion only — key check skipped"},
	{stem: "steel-folk", scale: pcSet(0, 2, 3, 5, 7, 9, 10), note: "G major (pentatonic-based)"},
	{stem: "electric-riff", scale: pcSet(0, 2, 3, 5, 7, 9, 10), note: "E minor pentatonic-based"},
	{stem: "bass-groove", scale: pcSet(0, 2, 3, 5, 7, 8, 10), note: "A minor"},

	// ── MusicXML-sourced ──
	{stem: "asturias", scale: pcSet(0, 2, 3, 5, 7, 9, 10), note: "E minor (Albéniz)"},
}

func TestTemplatesComplyWithSourceOfTruth(t *testing.T) {
	for _, mt := range musicTruths {
		t.Run(mt.stem, func(t *testing.T) {
			s := showcaseByStem(t, mt.stem)
			if mt.bpm > 0 && absInt(s.BPM-mt.bpm) > mt.bpmTol {
				t.Errorf("%s BPM %d is outside canonical %d±%d (%s)", mt.stem, s.BPM, mt.bpm, mt.bpmTol, mt.note)
			}
			if mt.scale == nil {
				return
			}
			for ri, in := range s.Insts {
				if !pitchedInstrument(in.ID) {
					continue
				}
				for _, h := range s.Rows[ri].Hits {
					pc := ((int(h.Pitch) % 12) + 12) % 12
					if !mt.scale[pc] {
						t.Errorf("row %q (%s): pitch %d (class %d) is not in key — %s", in.Name, in.ID, int(h.Pitch), pc, mt.note)
					}
				}
			}
		})
	}
}

// TestAfricaChorusHasFlatSeven pins the defining harmonic colour of Toto's
// "Africa" chorus: the A-major bVII chord (A-G#m-C#m-B loop). A diatonic check
// alone misses this because B-F#-G#m-E (the earlier draft) is also inside B major
// — only the presence of the A-natural (pc 0) ♭VII root distinguishes the correct
// progression.
func TestAfricaChorusHasFlatSeven(t *testing.T) {
	s := showcaseByStem(t, "toto-africa")
	for ri, in := range s.Insts {
		if in.ID != "piano-grand" && in.ID != "bass-guitar" {
			continue
		}
		for _, h := range s.Rows[ri].Hits {
			if (((int(h.Pitch) % 12) + 12) % 12) == 0 { // A natural
				return
			}
		}
	}
	t.Fatal("toto-africa harmony has no A-natural (bVII) — the chorus loop must be A-G#m-C#m-B, not B-F#-G#m-E")
}
