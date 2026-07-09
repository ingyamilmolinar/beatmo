package main

// Batch 3 — the classical templates, rebuilt from REAL public-domain scores
// (2026-07 enrichment): music21-corpus / KernScores-mirror / IMSLP MusicXML,
// trimmed to each piece's recognizable opening (scores/MANIFEST in the
// research scratchpad documents every source URL). Parsed at generation time
// via internal/musicxml + scoreToShowcaseMulti — the notes ARE the score.
//
// Octave policy: cello/cello-warm render one octave below written pitch, so
// their rows are transposed +12 (same rule as the bass instruments).
// Transposing brass (Handel arrangement): horn parts are written +7 above
// sounding, Bb trumpets +2 — countered per row.

import (
	_ "embed"

	"github.com/ingyamilmolinar/beatmo/internal/musicxml"
)

//go:embed scores/bach-prelude-c-16.musicxml
var preludeCXML []byte

//go:embed scores/bach-cello-prelude-8.musicxml
var celloPreludeXML []byte

//go:embed scores/bach-flute-allemande-8.musicxml
var fluteAllemandeXML []byte

//go:embed scores/bach-toccata-12.musicxml
var toccataXML []byte

//go:embed scores/mozart-k545-12.musicxml
var k545XML []byte

//go:embed scores/pachelbel-canon-16.musicxml
var pachelbelXML []byte

//go:embed scores/vivaldi-spring-13.musicxml
var vivaldiXML []byte

//go:embed scores/marcello-adagio-16.musicxml
var marcelloXML []byte

//go:embed scores/handel-hornpipe-16.musicxml
var handelXML []byte

func mustScore(data []byte, name string) musicxml.Score {
	sc, err := musicxml.Parse(data)
	if err != nil {
		panic("gen-showcase: parse " + name + ": " + err.Error())
	}
	if len(sc.Parts) == 0 {
		panic("gen-showcase: " + name + ": no parts")
	}
	return sc
}

// selectParts returns a copy of sc keeping only the parts at the given indices.
func selectParts(sc musicxml.Score, idx ...int) musicxml.Score {
	out := sc
	out.Parts = nil
	for _, i := range idx {
		out.Parts = append(out.Parts, sc.Parts[i])
	}
	return out
}

// transposeRow shifts every hit of row ri by semis (octave-policy fixes and
// transposing-instrument corrections).
func transposeRow(sh Showcase, ri int, semis float64) Showcase {
	sh.Rows[ri].Hits = transposeHits(sh.Rows[ri].Hits, semis)
	return sh
}

// ── Bach — Prelude in C, BWV 846 (16 bars, music21 corpus) ──────────────────
func preludeCShowcase() Showcase {
	sh := scoreToShowcaseMulti(mustScore(preludeCXML, "bwv846"), "bach-prelude-c", 16, []InstSpec{
		{ID: "piano-grand", Name: "Piano", Volume: 0.8, ReverbSend: 0.16},
	})
	sh.BPM = 66
	sh = capDuplicateIDs(sh, classicalFallbacks)
	return sh
}

// ── Bach — Cello Suite 1 Prelude, BWV 1007 (8 bars, KernScores mirror) ──────
func celloPreludeShowcase() Showcase {
	sh := scoreToShowcaseMulti(mustScore(celloPreludeXML, "bwv1007"), "bach-cello-prelude", 16, []InstSpec{
		{ID: "cello", Name: "Cello", Volume: 0.78, ReverbSend: 0.24},
	})
	sh.BPM = 68
	for ri := range sh.Rows {
		sh = transposeRow(sh, ri, 12) // cello renders -1 octave
	}
	sh = capDuplicateIDs(sh, classicalFallbacks)
	return sh
}

// ── Bach — Flute Partita Allemande, BWV 1013 (8 bars, freedots MusicXML) ────
func fluteAllemandeShowcase() Showcase {
	sh := scoreToShowcaseMulti(mustScore(fluteAllemandeXML, "bwv1013"), "bach-flute-allemande", 16, []InstSpec{
		{ID: "flute", Name: "Flute", Volume: 0.75, ReverbSend: 0.22},
	})
	sh.BPM = 88
	sh = capDuplicateIDs(sh, classicalFallbacks)
	return sh
}

// ── Bach — Toccata in D minor, BWV 565 (12 bars, KernScores mirror) ─────────
func toccataShowcase() Showcase {
	sh := scoreToShowcaseMulti(mustScore(toccataXML, "bwv565"), "bach-toccata", 16, []InstSpec{
		{ID: "organ-church", Name: "Manual I", Volume: 0.72, Pan: -0.1, ReverbSend: 0.42},
		{ID: "organ-church", Name: "Manual II", Volume: 0.6, Pan: 0.1, ReverbSend: 0.42},
		{ID: "organ-church", Name: "Pedal", Volume: 0.7, ReverbSend: 0.36},
	})
	sh.BPM = 70
	sh = capDuplicateIDs(sh, classicalFallbacks)
	return sh
}

// ── Mozart — Sonata K 545, mvt 1 (12 bars, craigsapp kern) ──────────────────
func k545Showcase() Showcase {
	sh := scoreToShowcaseMulti(mustScore(k545XML, "k545"), "mozart-k545", 16, []InstSpec{
		{ID: "piano-grand", Name: "Right Hand", Volume: 0.76, ReverbSend: 0.14},
		{ID: "piano-grand", Name: "Left Hand", Volume: 0.58, ReverbSend: 0.14},
	})
	sh.BPM = 120
	sh = capDuplicateIDs(sh, classicalFallbacks)
	return sh
}

// ── Pachelbel — Canon in D (16 bars, KernScores mirror) ─────────────────────
// Three canon violins over the harpsichord ground (RH → harp, LH → cello).
func pachelbelShowcase() Showcase {
	sh := scoreToShowcaseMulti(mustScore(pachelbelXML, "canon"), "pachelbel-violin", 16, []InstSpec{
		{ID: "violin", Name: "Violin I", Volume: 0.72, Pan: -0.3, ReverbSend: 0.26},
		{ID: "violin", Name: "Violin II", Volume: 0.64, ReverbSend: 0.26},
		{ID: "violin", Name: "Violin III", Volume: 0.58, Pan: 0.3, ReverbSend: 0.26},
		{ID: "harp", Name: "Continuo", Volume: 0.5, Pan: 0.15, ReverbSend: 0.2},
		{ID: "cello-warm", Name: "Ground", Volume: 0.62, Pan: -0.15, ReverbSend: 0.2},
	})
	sh.BPM = 64
	// The parser may split harpsichord voices into extra rows; transpose every
	// cello-warm row up an octave (renders -1 octave).
	for ri, r := range sh.Rows {
		if r.Inst == "cello-warm" {
			sh = transposeRow(sh, ri, 12)
		}
	}
	sh = capDuplicateIDs(sh, classicalFallbacks)
	return sh
}

// ── Vivaldi — Spring RV 269, mvt 1 (13 bars, harshshredding kern) ───────────
func vivaldiShowcase() Showcase {
	sh := scoreToShowcaseMulti(mustScore(vivaldiXML, "rv269"), "vivaldi-spring", 16, []InstSpec{
		{ID: "violin", Name: "Solo Violin", Volume: 0.75, ReverbSend: 0.24},
		{ID: "violin-ensemble", Name: "Violins I", Volume: 0.55, Pan: -0.25, ReverbSend: 0.28},
		{ID: "violin-ensemble", Name: "Violins II", Volume: 0.5, Pan: 0.25, ReverbSend: 0.28},
		{ID: "viola-pad", Name: "Violas", Volume: 0.48, Pan: 0.1, ReverbSend: 0.26},
		{ID: "cello", Name: "Continuo", Volume: 0.6, Pan: -0.1, ReverbSend: 0.22},
	})
	sh.BPM = 110
	for ri, r := range sh.Rows {
		if r.Inst == "cello" {
			sh = transposeRow(sh, ri, 12)
		}
	}
	sh = capDuplicateIDs(sh, classicalFallbacks)
	return sh
}

// ── Marcello — Oboe Concerto Adagio (16 bars, IMSLP engraving) ──────────────
func marcelloShowcase() Showcase {
	sh := scoreToShowcaseMulti(mustScore(marcelloXML, "marcello"), "marcello-oboe", 16, []InstSpec{
		{ID: "oboe-full", Name: "Oboe", Volume: 0.78, ReverbSend: 0.26},
		{ID: "violin-ensemble", Name: "Violins I", Volume: 0.42, Pan: -0.25, ReverbSend: 0.28},
		{ID: "violin-ensemble", Name: "Violins II", Volume: 0.4, Pan: 0.25, ReverbSend: 0.28},
		{ID: "cello", Name: "Bass", Volume: 0.55, Pan: -0.1, ReverbSend: 0.22},
		{ID: "harp", Name: "Continuo", Volume: 0.4, Pan: 0.15, ReverbSend: 0.2},
	})
	sh.BPM = 54
	// This IMSLP engraving is in G minor; shift -5 to the concerto's
	// canonical D minor (cello additionally +12 for its octave-down render).
	for ri, r := range sh.Rows {
		if r.Inst == "cello" {
			sh = transposeRow(sh, ri, 7)
		} else {
			sh = transposeRow(sh, ri, -5)
		}
	}
	sh = capDuplicateIDs(sh, classicalFallbacks)
	return sh
}

// ── Handel — Water Music, Alla Hornpipe HWV 349 (16 bars, IMSLP brass) ──────
// Subset of the 11-part brass arrangement: Hn 1 (written +7), Tpt 1/2
// (written +2), Tbn 1 + Tuba (concert). Transpositions countered per row.
func handelShowcase() Showcase {
	sc := selectParts(mustScore(handelXML, "hwv349"), 0, 2, 3, 6, 10)
	sh := scoreToShowcaseMulti(sc, "handel-water-horn", 16, []InstSpec{
		{ID: "french-horn", Name: "Horn", Volume: 0.6, Pan: -0.2, ReverbSend: 0.28},
		{ID: "trumpet", Name: "Trumpet I", Volume: 0.72, Pan: 0.15, ReverbSend: 0.24},
		{ID: "trumpet-mellow", Name: "Trumpet II", Volume: 0.6, Pan: 0.3, ReverbSend: 0.24},
		{ID: "french-horn-loud", Name: "Trombone", Volume: 0.52, Pan: -0.3, ReverbSend: 0.24},
		{ID: "french-horn-loud", Name: "Tuba", Volume: 0.55, ReverbSend: 0.2},
	})
	sh.BPM = 120
	// The brass arrangement SOUNDS in Ab major (measured: all 305 concert
	// pitches fit Ab) — a tritone from the original D. Per-row offsets fold
	// the instrument transposition (horn -7, Bb tpt -2) together with the
	// Ab→D shift, choosing the tritone direction that keeps each register
	// sensible (trumpets up, low brass down).
	for ri, r := range sh.Rows {
		switch r.Inst {
		case "french-horn":
			sh = transposeRow(sh, ri, -1) // -7 transpose, +6 to D
		case "trumpet", "trumpet-mellow":
			sh = transposeRow(sh, ri, 4) // -2 transpose, +6 to D
		case "french-horn-loud":
			sh = transposeRow(sh, ri, -6) // concert, -6 to D
		}
	}
	sh = capDuplicateIDs(sh, classicalFallbacks)
	return sh
}

// capDuplicateIDs reassigns 3rd-and-later rows of the same instrument id to a
// registered sibling timbre (the auto "-N" suffix is only valid for N=2).
// Multi-voice score parts produce one row per voice, so a 3-voice piano part
// would otherwise emit "piano-grand-3".
func capDuplicateIDs(sh Showcase, fallback map[string]string) Showcase {
	seen := map[string]int{}
	for i := range sh.Insts {
		id := sh.Insts[i].ID
		seen[id]++
		if seen[id] >= 3 {
			fb, ok := fallback[id]
			if !ok {
				panic("gen-showcase: " + sh.Stem + ": >2 rows of " + id + " and no fallback")
			}
			sh.Insts[i].ID = fb
			sh.Rows[i].Inst = fb
			seen[fb]++
		}
	}
	return sh
}

var classicalFallbacks = map[string]string{
	"organ-church": "organ",
	"piano-grand":  "piano-felt",
	"violin":       "violin-ensemble",
	"harp":         "guitar-nylon",
	"violin-ensemble": "viola-pad",
	"cello":        "cello-warm",
}
