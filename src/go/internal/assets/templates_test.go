package assets

import (
	"encoding/json"
	"testing"
)

// The seven built-in templates are authoritative JSON project files (no
// procedural generator). These guards validate the embedded files directly,
// the same way the startup demo is validated — see internal/ui startup_demo_*
// tests for the import/quality coverage of the actual circuits.

func TestTemplates_OrderedAndNonEmpty(t *testing.T) {
	tpls := Templates()
	wantGenres := []string{"rock", "hip-hop", "pop", "funk", "salsa", "house", "techno"}
	if len(tpls) != len(wantGenres) {
		t.Fatalf("got %d templates, want %d", len(tpls), len(wantGenres))
	}
	for i, tp := range tpls {
		if tp.Genre != wantGenres[i] {
			t.Fatalf("template %d genre=%q want %q", i, tp.Genre, wantGenres[i])
		}
		if tp.Display == "" {
			t.Fatalf("template %q missing display name", tp.Genre)
		}
		if len(tp.Bytes) == 0 {
			t.Fatalf("template %q has no bytes", tp.Genre)
		}
		var doc struct {
			Version int `json:"version"`
			BPM     int `json:"bpm"`
		}
		if err := json.Unmarshal(tp.Bytes, &doc); err != nil {
			t.Fatalf("template %q bytes not JSON: %v", tp.Genre, err)
		}
		if doc.Version != 1 {
			t.Fatalf("template %q version=%d want 1", tp.Genre, doc.Version)
		}
		// BPM is derived from the JSON, so it must equal the file's bpm field.
		if tp.BPM != doc.BPM {
			t.Fatalf("template %q BPM=%d but JSON bpm=%d", tp.Genre, tp.BPM, doc.BPM)
		}
		if tp.BPM <= 0 {
			t.Fatalf("template %q bpm=%d (must be > 0)", tp.Genre, tp.BPM)
		}
	}
}

// Display names are derived from the file stem (no hand-maintained table), so a
// hyphenated stem must title-case every segment.
func TestTemplates_DisplayDerivedFromStem(t *testing.T) {
	want := map[string]string{
		"rock": "Rock", "hip-hop": "Hip-Hop", "pop": "Pop", "funk": "Funk",
		"salsa": "Salsa", "house": "House", "techno": "Techno",
	}
	for _, tp := range Templates() {
		if got := tp.Display; got != want[tp.Genre] {
			t.Errorf("%s: display=%q want %q", tp.Genre, got, want[tp.Genre])
		}
	}
}

// BPMs must stay in each genre's authentic band (a template with a wildly wrong
// tempo would play unrecognizably). Bands are deliberately wide; the exact value
// is whatever the shipped JSON carries.
func TestTemplates_BPMInGenreBand(t *testing.T) {
	band := map[string]struct{ lo, hi int }{
		"rock":    {100, 140},
		"hip-hop": {80, 100},
		"pop":     {100, 132},
		"funk":    {95, 120},
		"salsa":   {130, 180},
		"house":   {118, 128},
		"techno":  {125, 150},
	}
	for _, tp := range Templates() {
		b, ok := band[tp.Genre]
		if !ok {
			t.Fatalf("%s: no expected BPM band — add one", tp.Genre)
		}
		if tp.BPM < b.lo || tp.BPM > b.hi {
			t.Errorf("%s: BPM=%d outside genre band [%d,%d]", tp.Genre, tp.BPM, b.lo, b.hi)
		}
	}
}
