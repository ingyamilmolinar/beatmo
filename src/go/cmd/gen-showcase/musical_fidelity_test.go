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

// TestOyeComoVaSonClave: the sidestick "Clave" row must spell the canonical son
// clave 2-3 over its 2-bar (32-step) cycle — 2-side on beats 2 & 3 (steps 4, 8),
// 3-side on 1, "& of 2", 4 (steps 16, 22, 28). Clave is the backbone of salsa, so
// a misplaced stroke is a real, audible regression.
func TestOyeComoVaSonClave(t *testing.T) {
	row := rowByInstName(t, showcaseByStem(t, "puente-oye-como-va"), "Clave")
	want := map[int]bool{4: true, 8: true, 16: true, 22: true, 28: true}
	got := map[int]bool{}
	for _, h := range row.Hits {
		got[h.Step%32] = true
	}
	if len(got) != len(want) {
		t.Fatalf("clave has %d distinct steps-in-cycle, want %d (%v)", len(got), len(want), got)
	}
	for s := range want {
		if !got[s] {
			t.Errorf("clave missing son-clave 2-3 stroke at step %d (got %v)", s, got)
		}
	}
	// Explicitly guard against the prior wrong placement.
	for _, bad := range []int{10, 19} {
		if got[bad] {
			t.Errorf("clave has stroke at step %d — not a son-clave position (regression)", bad)
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
