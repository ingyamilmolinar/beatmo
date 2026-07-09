package main

import "testing"

// These tests pin musically-defining properties of specific templates that are
// easy to regress silently (a wrong clave step or a non-diatonic melody note
// still "builds" fine but stops sounding like the song). They assert at the
// spec level (showcases()), which is the source of truth that build() renders.

// showcaseByStem returns the showcase with the given stem, failing the test if
// it is absent.
func showcaseByStem(t *testing.T, stem string) Showcase {
	t.Helper()
	for _, s := range showcases() {
		if s.Stem == stem {
			return s
		}
	}
	t.Fatalf("showcase %q not found", stem)
	return Showcase{}
}

// rowByInstName returns the row whose 1:1 InstSpec carries the given Name.
func rowByInstName(t *testing.T, s Showcase, name string) RowSpec {
	t.Helper()
	for i, in := range s.Insts {
		if in.Name == name {
			return s.Rows[i]
		}
	}
	t.Fatalf("showcase %q has no instrument named %q", s.Stem, name)
	return RowSpec{}
}

// TestOyeComoVaChaChaBell: cha-cha-cha has NO clave and NO cascara (Liberty
// Park Music pedagogy + the source MIDI, 2026-07 dossier) — the timekeeper is
// the quarter-note cha-cha bell. The old son-clave row was a salsa-ism; this
// pins the corrected genre convention: bell quarters present, no "Clave" row.
func TestOyeComoVaChaChaBell(t *testing.T) {
	s := showcaseByStem(t, "puente-oye-como-va")
	for _, in := range s.Insts {
		if in.Name == "Clave" {
			t.Fatal("cha-cha-cha must not carry a clave row (genre fix, 2026-07)")
		}
	}
	bell := rowByInstName(t, s, "Cha Bell")
	got := map[int]bool{}
	for _, h := range bell.Hits {
		got[h.Step%16] = true
	}
	for _, q := range []int{0, 4, 8, 12} {
		if !got[q] {
			t.Errorf("cha-cha bell missing quarter pulse at in-bar step %d", q)
		}
	}
}

// fMajorPitchClasses are the semitone classes (mod 12, from A3=0) of F major:
// F G A Bb C D E — PLUS B-natural (class 2), the chromatic passing/leading tone
// that the verbatim trillian.mit.edu ABC places in A-section bar 6 (B,→C, over
// the G7). This matches the source-of-truth scale for jobim-ipanema ("F major
// (+B from G7)"). Used to assert the Ipanema melody is in key.
var fMajorPitchClasses = map[int]bool{8: true, 10: true, 0: true, 1: true, 2: true, 3: true, 5: true, 7: true}

// TestIpanemaSaxDiatonicToFMajor: the sax melody plays over Fmaj7/G7, so every
// note must be diatonic to F major (plus the documented B-natural leading tone).
// The earlier draft used G# (pitch 11) and F# (pitch 9), both of which clash with
// the underlying harmony — those remain forbidden.
func TestIpanemaSaxDiatonicToFMajor(t *testing.T) {
	row := rowByInstName(t, showcaseByStem(t, "jobim-ipanema"), "Sax")
	if len(row.Hits) == 0 {
		t.Fatal("ipanema sax has no hits")
	}
	for _, h := range row.Hits {
		pc := ((int(h.Pitch) % 12) + 12) % 12
		if !fMajorPitchClasses[pc] {
			t.Errorf("sax pitch %d (class %d) is not diatonic to F major — clashes with Fmaj7/G7", int(h.Pitch), pc)
		}
	}
}

// TestGoodTimesArrangement pins the enriched Good Times arc: the verbatim
// Edwards chromatic walk (G#1→A1, C#2→D2), guitars tacet from the chant
// turnaround on (bar 9+), and bass carrying the full 16 bars.
func TestGoodTimesArrangement(t *testing.T) {
	s := showcaseByStem(t, "chic-good-times")
	if s.Bars != 16 {
		t.Fatalf("Bars = %d, want 16", s.Bars)
	}
	bass := rowByInstName(t, s, "Bass")
	var hasGsharp, hasCsharp, bassInBreakdown bool
	for _, h := range bass.Hits {
		if h.Pitch == -13 { // G#2 encoded (renders G#1 — bass renders -1 octave)
			hasGsharp = true
		}
		if h.Pitch == -8 { // C#3 encoded (renders C#2)
			hasCsharp = true
		}
		if h.Step >= 192 {
			bassInBreakdown = true
		}
	}
	if !hasGsharp || !hasCsharp {
		t.Errorf("bass lost the chromatic walk (G#1 %v, C#2 %v)", hasGsharp, hasCsharp)
	}
	if !bassInBreakdown {
		t.Error("bass must carry the breakdown (bars 13-16)")
	}
	for _, name := range []string{"Chuck", "Funk Gtr"} {
		for _, h := range rowByInstName(t, s, name).Hits {
			if h.Step >= 128 {
				t.Fatalf("%s has a hit at step %d — guitars are tacet from bar 9 (chant + breakdown)", name, h.Step)
			}
		}
	}
}

// TestSuperstitionSwingAndStack pins the enriched Superstition: the 2:1
// triplet 16th swing (every off-16th clav/bass hit carries delay groove 0.33),
// the D#1 clav anchor, and horns confined to the chorus (bars 13-16).
func TestSuperstitionSwingAndStack(t *testing.T) {
	s := showcaseByStem(t, "wonder-superstition")
	clav := rowByInstName(t, s, "Clav 1")
	var anchors int
	for _, h := range clav.Hits {
		if h.Pitch == -30 {
			anchors++
		}
		if h.Step%2 == 1 && (h.Groove != "delay" || h.GroovePct != 0.33) {
			t.Fatalf("clav1 off-16th at step %d lost its swing (groove=%q pct=%v)", h.Step, h.Groove, h.GroovePct)
		}
	}
	if anchors < 8 {
		t.Errorf("clav1 has %d D#1 anchors, want >=8", anchors)
	}
	for _, name := range []string{"Trumpet", "Tenor Sax"} {
		for _, h := range rowByInstName(t, s, name).Hits {
			if h.Step < 192 {
				t.Fatalf("%s hit at step %d — horns enter at the chorus (step 192)", name, h.Step)
			}
		}
	}
}

// TestExodusArrangement pins the enriched Exodus: the chromatic G#1 walk-up in
// Familyman's 4-bar cycle, the laid-back clav bubble (measured +13-17ms in the
// source MIDI → delay groove), and the three-part horn answer.
func TestExodusArrangement(t *testing.T) {
	s := showcaseByStem(t, "marley-exodus")
	var gsharp int
	for _, h := range rowByInstName(t, s, "Bass").Hits {
		if h.Pitch == -13 { // G#2 encoded (renders G#1 — bass renders -1 octave)
			gsharp++
		}
	}
	if gsharp < 8 {
		t.Errorf("bass has %d G# walk-up notes, want >=8 (2 per 4-bar cycle x4)", gsharp)
	}
	for _, h := range rowByInstName(t, s, "Bubble").Hits {
		if h.Groove != "delay" {
			t.Fatalf("clav bubble hit at step %d lost its laid-back groove", h.Step)
		}
	}
	for _, name := range []string{"Trumpet", "Alto", "Trombone"} {
		if len(rowByInstName(t, s, name).Hits) == 0 {
			t.Errorf("horn row %s is empty", name)
		}
	}
}
