package assets

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// TestTemplates_AllInstrumentsRegistered guards against templates referencing an
// instrument id that isn't a real built-in — such an id renders RED/absent in the
// UI (the "bass" vs real-id bug). Every instrument id in every template MUST
// resolve to a built-in in audio.BuiltinInstrumentIDs (the single source of truth
// for registered instruments, generated from engine_instruments.go).
//
// Instance variants are valid: gen-showcase auto-suffixes a duplicated id within
// a template (e.g. two organ voices → "organ" + "organ-2") so each voice carries
// its own display name, and audio.EnsureInstanceInstrument promotes the "-1"/"-2"
// variant at import time to render IDENTICALLY to its registered base. Such an id
// is valid iff its base (with the instance suffix stripped) is registered — this
// mirrors audio.instanceBaseID, the canonical strip.
func TestTemplates_AllInstrumentsRegistered(t *testing.T) {
	registered := make(map[string]bool, len(audio.BuiltinInstrumentIDs))
	for _, id := range audio.BuiltinInstrumentIDs {
		registered[id] = true
	}
	// instanceBaseStrip mirrors audio.instanceBaseID (unexported): strip a
	// trailing "-1"/"-2" instance suffix so the variant resolves to its base.
	instanceBaseStrip := func(id string) string {
		if len(id) > 2 && (strings.HasSuffix(id, "-1") || strings.HasSuffix(id, "-2")) {
			return id[:len(id)-2]
		}
		return id
	}
	for _, tp := range Templates() {
		var doc struct {
			Instruments []struct {
				ID string `json:"id"`
			} `json:"instruments"`
		}
		if err := json.Unmarshal(tp.Bytes, &doc); err != nil {
			t.Fatalf("template %q: unmarshal: %v", tp.Genre, err)
		}
		if len(doc.Instruments) == 0 {
			t.Errorf("template %q has no instruments", tp.Genre)
		}
		for _, in := range doc.Instruments {
			if !registered[in.ID] && !registered[instanceBaseStrip(in.ID)] {
				t.Errorf("template %q references UNREGISTERED instrument %q (renders RED). "+
					"Use a real id from audio.BuiltinInstrumentIDs (e.g. bass → bass-guitar/sub-bass/fm-bass)",
					tp.Genre, in.ID)
			}
		}
	}
}

// The 15 built-in templates are authoritative JSON project files, each recreating
// the instrumental signature of one genre-defining masterpiece. All are generated
// by cmd/gen-showcase (regenerate via `go run ./cmd/gen-showcase`). These guards
// validate the embedded files directly, the same way the startup demo is validated
// — see internal/ui startup_demo_* tests for the import/quality coverage.

func TestTemplates_OrderedAndNonEmpty(t *testing.T) {
	tpls := Templates()
	wantGenres := []string{
		"bach-toccata", "vivaldi-spring", "mozart-k545", "bach-prelude-c", "bach-flute-allemande", "asturias",
		"bach-cello-prelude", "pachelbel-violin", "marcello-oboe", "cielito-trumpet", "handel-water-horn",
		"felt-prelude", "sax-blues", "steel-folk", "electric-riff", "clav-funk", "bass-groove", "conga-tumbao",
		"jobim-ipanema", "miles-so-what", "bbking-thrill-is-gone",
		"wonder-superstition", "chic-good-times",
		"eagles-hotel-california", "marley-exodus", "toto-africa",
		"dre-g-thang", "blackbox-ride-on-time", "gaynor-survive",
		"puente-oye-como-va", "salsa-vivir", "bachata-obsesion",
	}
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

// Display names are the curated "Artist — Song" titles, not title-cased stems —
// the menu names the original each template recreates.
func TestTemplates_DisplayNamed(t *testing.T) {
	want := map[string]string{
		"bach-toccata":            "Bach — Toccata & Fugue in D minor",
		"vivaldi-spring":          "Vivaldi — Spring",
		"mozart-k545":             "Mozart — Sonata K545",
		"bach-prelude-c":          "Bach — Prelude in C (BWV 846)",
		"bach-flute-allemande":    "Bach — Flute Partita Allemande (BWV 1013)",
		"asturias":                "Albéniz — Asturias (Leyenda)",
		"bach-cello-prelude":      "Bach — Cello Suite 1 Prelude (BWV 1007)",
		"pachelbel-violin":        "Pachelbel — Canon in D — Violin",
		"marcello-oboe":           "Marcello — Oboe Concerto in D minor, Adagio",
		"cielito-trumpet":         "Cielito Lindo (Mendoza, 1882) — Mariachi Trumpet",
		"handel-water-horn":       "Handel — Water Music, Bourrée — French Horn",
		"felt-prelude":            "Felt Piano — Bach Prelude (soft)",
		"sax-blues":               "Blues Line — Alto Sax",
		"steel-folk":              "Folk Fingerpick — Steel Guitar",
		"electric-riff":           "Pentatonic Riff — Electric Guitar",
		"clav-funk":               "Funk Riff — Guitar",
		"bass-groove":             "Bass Groove — Synth Bass",
		"conga-tumbao":            "Tumbao — Congas",
		"jobim-ipanema":           "Jobim — The Girl from Ipanema",
		"miles-so-what":           "Miles Davis — So What",
		"bbking-thrill-is-gone":   "B.B. King — The Thrill Is Gone",
		"wonder-superstition":     "Stevie Wonder — Superstition",
		"chic-good-times":         "Chic — Good Times",
		"eagles-hotel-california": "Eagles — Hotel California",
		"marley-exodus":           "Bob Marley — Exodus",
		"toto-africa":             "Toto — Africa",
		"dre-g-thang":             "Dr. Dre — Nuthin' but a 'G' Thang",
		"blackbox-ride-on-time":   "Black Box — Ride On Time",
		"gaynor-survive":          "Gloria Gaynor — I Will Survive",
		"puente-oye-como-va":      "Tito Puente — Oye Como Va",
		"salsa-vivir":             "Salsa — Vivir Mi Vida",
		"bachata-obsesion":        "Bachata — Obsesión",
	}
	for _, tp := range Templates() {
		if got := tp.Display; got != want[tp.Genre] {
			t.Errorf("%s: display=%q want %q", tp.Genre, got, want[tp.Genre])
		}
		// Every shipped template must have a curated title, never the title-cased
		// fallback (which would contain no space / em-dash).
		if !strings.Contains(tp.Display, "—") {
			t.Errorf("%s: display=%q looks like the title-cased fallback — add a displayTitles entry", tp.Genre, tp.Display)
		}
	}
}

// TestTemplates_DirectoryHasNoOrphans guards the embed: //go:embed templates/*.json
// pulls in EVERY .json in the directory, but only those in templateOrder are
// served. A leftover old/stale file would silently bloat the binary, so the set
// of embedded files must equal templateOrder exactly.
func TestTemplates_DirectoryHasNoOrphans(t *testing.T) {
	entries, err := templateFS.ReadDir("templates")
	if err != nil {
		t.Fatalf("read embedded templates dir: %v", err)
	}
	got := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		got[strings.TrimSuffix(name, ".json")] = true
	}
	want := map[string]bool{}
	for _, s := range templateOrder {
		want[s] = true
	}
	for s := range got {
		if !want[s] {
			t.Errorf("embedded templates/%s.json is not in templateOrder — orphan file (delete it or register it)", s)
		}
	}
	for s := range want {
		if !got[s] {
			t.Errorf("templateOrder lists %q but templates/%s.json is not embedded", s, s)
		}
	}
}

// BPMs must stay in each song's authentic band (a template with a wildly wrong
// tempo would play unrecognizably). Bands are deliberately wide; the exact value
// is whatever the shipped JSON carries.
func TestTemplates_BPMInGenreBand(t *testing.T) {
	band := map[string]struct{ lo, hi int }{
		"bach-toccata":            {60, 90},
		"vivaldi-spring":          {95, 125},
		"mozart-k545":             {105, 135},
		"bach-prelude-c":          {55, 80},
		"bach-flute-allemande":    {75, 100},
		"asturias":                {95, 130},
		"bach-cello-prelude":      {55, 80},
		"pachelbel-violin":        {55, 80},
		"marcello-oboe":           {45, 70},
		"cielito-trumpet":         {150, 180},
		"handel-water-horn":       {100, 135},
		"felt-prelude":            {50, 75},
		"sax-blues":               {80, 115},
		"steel-folk":              {75, 110},
		"electric-riff":           {100, 140},
		"clav-funk":               {85, 125},
		"bass-groove":             {90, 130},
		"conga-tumbao":            {80, 120},
		"jobim-ipanema":           {115, 140},
		"miles-so-what":           {120, 150},
		"bbking-thrill-is-gone":   {80, 110},
		"wonder-superstition":     {90, 115},
		"chic-good-times":         {105, 125},
		"eagles-hotel-california": {65, 90},
		// Exodus: MIDI tempo meta (bitmidi 18775) + songbpm.com both say ~132;
		// the old 98 was unsourced (2026-07 enrichment dossier).
		"marley-exodus": {125, 140},
		"toto-africa":             {85, 105},
		"dre-g-thang":             {80, 100},
		"blackbox-ride-on-time":   {115, 130},
		"gaynor-survive":          {110, 125},
		"puente-oye-como-va":      {110, 135},
		"salsa-vivir":             {95, 120},
		"bachata-obsesion":        {120, 145},
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
