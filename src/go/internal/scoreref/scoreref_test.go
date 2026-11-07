package scoreref

import (
	"crypto/sha256"
	"encoding/hex"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/musicxml"
)

func cat(t *testing.T) Cat {
	t.Helper()
	c, err := Catalogue()
	if err != nil {
		t.Fatalf("Catalogue: %v", err)
	}
	if len(c.Scores) == 0 {
		t.Fatal("catalogue is empty")
	}
	return c
}

// TestCatalogueIntegrity covers the structural invariants: unique ids, no
// blank required fields, and a 1:1 correspondence between entries and files.
func TestCatalogueIntegrity(t *testing.T) {
	c := cat(t)

	seenID := map[string]bool{}
	seenFile := map[string]bool{}
	seenTemplate := map[string]string{}
	for _, s := range c.Scores {
		if seenID[s.ID] {
			t.Errorf("duplicate score id %q", s.ID)
		}
		seenID[s.ID] = true

		if s.File == "" || s.SHA256 == "" {
			t.Errorf("score %q: file/sha256 must not be empty", s.ID)
		}
		if seenFile[s.File] {
			t.Errorf("score %q: file %q is claimed by another entry", s.ID, s.File)
		}
		seenFile[s.File] = true

		if s.Work.Title == "" || s.Work.Composer == "" {
			t.Errorf("score %q: work title/composer must not be empty", s.ID)
		}
		if s.Source.URL == "" || s.Source.Repo == "" {
			t.Errorf("score %q: source url/repo must not be empty", s.ID)
		}
		if len(s.Templates) == 0 {
			t.Errorf("score %q backs no template — remove it or wire it up", s.ID)
		}
		// A template draws on at most one melody score and one groove.
		role := "melody"
		if s.IsGroove() {
			role = "groove"
		}
		for _, tpl := range s.Templates {
			k := tpl + "/" + role
			if prev, ok := seenTemplate[k]; ok {
				t.Errorf("template %q is claimed by both %q and %q as its %s", tpl, prev, s.ID, role)
			}
			seenTemplate[k] = s.ID
		}
	}
}

// An uncatalogued file has no recorded licence, exactly what the admission rule prevents.
func TestNoOrphanScoreFiles(t *testing.T) {
	c := cat(t)
	want := map[string]bool{}
	for _, s := range c.Scores {
		want[s.File] = true
		if s.Companion != nil {
			want[s.Companion.File] = true
		}
	}
	files, err := Files()
	if err != nil {
		t.Fatalf("Files: %v", err)
	}
	for _, f := range files {
		if !want[f] {
			t.Errorf("scores/%s is committed but absent from catalogue.json "+
				"(every committed score must record its licence and provenance)", f)
		}
	}
	if len(files) != len(want) {
		t.Errorf("committed files=%d catalogued files=%d", len(files), len(want))
	}
}

// TestGate1_WorkIsPublicDomain is the copyright gate on the COMPOSITION.
// A permissive encoding licence says nothing about the underlying work.
func TestGate1_WorkIsPublicDomain(t *testing.T) {
	now := time.Now()
	for _, s := range cat(t).Scores {
		if why := s.Work.Gate1Failure(now); why != "" {
			t.Errorf("score %q (%s, %s): work fails the public-domain gate: %s",
				s.ID, s.Work.Composer, s.Work.Title, why)
		}
	}
}

// US publication + 95 (PD on 1 January of year 96) and EU life + 70 (1 January of year 71), moving with the calendar.
func TestGate1CutoffsFollowTheCalendar(t *testing.T) {
	cases := []struct {
		date       string
		pub, death int
	}{
		{"2025-12-31", 1929, 1954},
		{"2026-01-01", 1930, 1955},
		{"2026-10-02", 1930, 1955},
		{"2031-06-15", 1935, 1960},
	}
	for _, c := range cases {
		d, _ := time.Parse("2006-01-02", c.date)
		if got := PublicationCutoff(d); got != c.pub {
			t.Errorf("%s: publication cut-off %d, want %d", c.date, got, c.pub)
		}
		if got := DeathCutoff(d); got != c.death {
			t.Errorf("%s: death cut-off %d, want %d", c.date, got, c.death)
		}
	}
}

// TestGate1CoversEveryAuthor: a work is free only when its LAST author's term
// has run, and a traditional work is admitted on a citation, never silently.
func TestGate1CoversEveryAuthor(t *testing.T) {
	now, _ := time.Parse("2006-01-02", "2026-10-02")
	base := Work{Title: "x", Composer: "c", FirstPublicationYear: 1900, ComposerDeathYear: 1920}
	if why := base.Gate1Failure(now); why != "" {
		t.Fatalf("baseline work fails: %s", why)
	}
	w := base
	w.Lyricist, w.LyricistDeathYear = "late lyricist", 1973
	if w.WorkIsPublicDomain(now) {
		t.Error("a song whose lyricist died in 1973 passed Gate 1")
	}
	w = base
	w.Arranger = "unknown arranger"
	if w.WorkIsPublicDomain(now) {
		t.Error("an arranger with no recorded death year passed Gate 1")
	}
	orig := Work{Title: "x", Composer: "Somebody", Original: true}
	if orig.WorkIsPublicDomain(now) {
		t.Error("an original credited to someone other than Beatmo passed Gate 1")
	}
	orig.Composer = OriginalComposer
	if why := orig.Gate1Failure(now); why != "" {
		t.Errorf("Beatmo original fails: %s", why)
	}
	if (Work{Title: "x", Composer: "y", Rhythm: true}).WorkIsPublicDomain(now) {
		t.Error("a rhythm pattern with no transcription reference passed Gate 1")
	}
	trad := Work{Title: "folk", Composer: "Traditional", Traditional: true}
	if trad.WorkIsPublicDomain(now) {
		t.Error("a traditional work with no reference passed Gate 1")
	}
	trad.Reference = "https://en.wikipedia.org/wiki/Clave_(rhythm)"
	if why := trad.Gate1Failure(now); why != "" {
		t.Errorf("cited traditional work fails: %s", why)
	}
	base.FirstPublicationYear = 1931
	if base.WorkIsPublicDomain(now) {
		t.Error("a 1931 publication passed in 2026")
	}
}

// TestGate2_LicenseAllowed is the licence gate on the ENCODING.
func TestGate2_LicenseAllowed(t *testing.T) {
	for _, s := range cat(t).Scores {
		if !LicenseAllowed(s.Source.License) {
			t.Errorf("score %q: licence %q is not on the allowlist %v "+
				"(NonCommercial and ShareAlike licences are excluded by policy — see docs/score-sources.md)",
				s.ID, s.Source.License, AllowedLicenses)
		}
		if s.Source.License == "CC-BY-4.0" && strings.TrimSpace(s.Source.Attribution) == "" {
			t.Errorf("score %q: CC-BY-4.0 requires a non-empty attribution", s.ID)
		}
	}
}

// The catalogue is hand-written and the file is not: the score's own <rights> must not contradict the claimed licence.
func TestGate2_EmbeddedRightsCorroborateLicense(t *testing.T) {
	for _, s := range cat(t).Scores {
		if s.FormatOrDefault() == FormatMIDI {
			corroborateFromLilyPond(t, s)
			continue
		}
		raw, err := Load(s.ID)
		if err != nil {
			t.Errorf("score %q: Load: %v", s.ID, err)
			continue
		}
		sc, err := musicxml.Parse(raw)
		if err != nil {
			t.Errorf("score %q: parse: %v", s.ID, err)
			continue
		}
		if sc.Rights != s.Source.EmbeddedRights {
			t.Errorf("score %q: catalogue records embedded_rights %q but the file says %q",
				s.ID, s.Source.EmbeddedRights, sc.Rights)
		}
		if sc.Rights == "" {
			t.Errorf("score %q: file carries no <rights> element — provenance unverifiable", s.ID)
			continue
		}
		low := strings.ToLower(sc.Rights)
		switch {
		case strings.Contains(low, "cc0"), strings.Contains(low, "public domain"):
			// consistent with CC0-1.0 / PD
		default:
			t.Errorf("score %q: <rights>%s</rights> does not corroborate licence %q",
				s.ID, sc.Rights, s.Source.License)
		}
		// An NC or SA claim on the SCORE disqualifies it; Hauptstimme's CC-By-SA covers only its analysis annotations (unused),
		// so the check is scoped to the score clause.
		scoreClause := low
		if i := strings.Index(low, "annotation"); i >= 0 {
			scoreClause = low[:i]
		}
		if strings.Contains(scoreClause, "noncommercial") || strings.Contains(scoreClause, "-nc") {
			t.Errorf("score %q: score rights claim is NonCommercial: %q", s.ID, sc.Rights)
		}
	}
}

// corroborateFromLilyPond checks a MIDI edition's licence against the
// LilyPond source it was generated from: MIDI carries no rights statement of
// its own, so the companion is the only in-file evidence.
func corroborateFromLilyPond(t *testing.T, s Score) {
	t.Helper()
	ly, err := LoadCompanion(s.ID)
	if err != nil {
		t.Errorf("score %q: a MIDI edition must commit its LilyPond source: %v", s.ID, err)
		return
	}
	lic := LilyPondLicense(ly)
	if lic != s.Source.EmbeddedRights {
		t.Errorf("score %q: catalogue records embedded_rights %q but the LilyPond source says %q",
			s.ID, s.Source.EmbeddedRights, lic)
	}
	if got := LicenseFromLilyPond(lic); got != s.Source.License {
		t.Errorf("score %q: LilyPond licence %q maps to %q, catalogue claims %q",
			s.ID, lic, got, s.Source.License)
	}
}

// TestCompanionBytesMatchCatalogue extends Gate 3 to the companion sources.
func TestCompanionBytesMatchCatalogue(t *testing.T) {
	for _, s := range cat(t).Scores {
		if s.Companion == nil {
			if s.FormatOrDefault() == FormatMIDI {
				t.Errorf("score %q: MIDI edition without a companion source", s.ID)
			}
			continue
		}
		b, err := LoadCompanion(s.ID)
		if err != nil {
			t.Errorf("score %q: %v", s.ID, err)
			continue
		}
		sum := sha256.Sum256(b)
		if got := hex.EncodeToString(sum[:]); got != s.Companion.SHA256 {
			t.Errorf("score %q: companion %s hashes %s, catalogue says %s", s.ID, s.Companion.File, got, s.Companion.SHA256)
		}
		if !strings.HasPrefix(s.Companion.URL, "https://") {
			t.Errorf("score %q: companion url %q is not https", s.ID, s.Companion.URL)
		}
	}
}

// TestMIDIPickupMatchesLilyPondPartial: the pickup decides where every
// downbeat falls, and MIDI cannot state it. The catalogue's value must be the
// \partial the edition was engraved with.
func TestMIDIPickupMatchesLilyPondPartial(t *testing.T) {
	for _, s := range cat(t).Scores {
		if s.FormatOrDefault() != FormatMIDI {
			if s.Pickup != "" {
				t.Errorf("score %q: pickup is only meaningful for MIDI editions", s.ID)
			}
			continue
		}
		ly, err := LoadCompanion(s.ID)
		if err != nil {
			continue // reported by the companion tests
		}
		if got := LilyPondPickup(ly); got != s.Pickup {
			t.Errorf("score %q: catalogue pickup %q, LilyPond source has \\partial %q", s.ID, s.Pickup, got)
		}
	}
}

// TestBeatmoScoresAreSelfDescribing: an idiom score Beatmo authored must say
// so in its own <rights>, be CC0, and be traditional with a citation.
func TestBeatmoScoresAreSelfDescribing(t *testing.T) {
	for _, s := range cat(t).Scores {
		if s.Source.Provider != ProviderBeatmo {
			continue
		}
		if s.Source.License != "CC0-1.0" || s.Source.EmbeddedRights != "Beatmo (CC0)" {
			t.Errorf("score %q: Beatmo-authored scores are CC0 with <rights>Beatmo (CC0)</rights>; got %q / %q",
				s.ID, s.Source.License, s.Source.EmbeddedRights)
		}
		kinds := 0
		for _, k := range []bool{s.Work.Traditional, s.Work.Rhythm, s.Work.Original} {
			if k {
				kinds++
			}
		}
		if kinds != 1 {
			t.Errorf("score %q: a Beatmo score is exactly one of traditional, rhythm or original (got %d)", s.ID, kinds)
		}
		if (s.Work.Traditional || s.Work.Rhythm) && s.Work.Reference == "" {
			t.Errorf("score %q: a traditional tune or a rhythm pattern must cite its source", s.ID)
		}
		if s.Work.Original && s.Work.Composer != OriginalComposer {
			t.Errorf("score %q: an original is composed by %q, got %q", s.ID, OriginalComposer, s.Work.Composer)
		}
		if s.IsGroove() && s.Work.Original || !s.IsGroove() && s.Work.Rhythm {
			t.Errorf("score %q: grooves are idioms or rhythm patterns; originals are melodies", s.ID)
		}
		if !strings.HasSuffix(s.Source.URL, "/internal/scoreref/scores/"+s.File) {
			t.Errorf("score %q: url %q must point at the committed file itself", s.ID, s.Source.URL)
		}
	}
}

func TestLilyPondDurationQuarters(t *testing.T) {
	cases := map[string]float64{"": 0, "8": 0.5, "4": 1, "4.": 1.5, "8*2": 1, "16": 0.25, "2*3/4": 1.5, "8..": 0.875}
	for in, want := range cases {
		got, err := LilyPondDurationQuarters(in)
		if err != nil || got != want {
			t.Errorf("LilyPondDurationQuarters(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"x", "0", "8*", "8*1/0"} {
		if _, err := LilyPondDurationQuarters(bad); err == nil {
			t.Errorf("LilyPondDurationQuarters(%q) accepted", bad)
		}
	}
}

func TestLilyPondHeaderAndLicense(t *testing.T) {
	ly := []byte("\\header {\n  title = \"The Entertainer\"\n  license = \"Public Domain\"\n" +
		" copyright = \\markup { footer }\n}\nmusic = { \\partial 8*2 a8 b }\n")
	if h := LilyPondHeader(ly); h["title"] != "The Entertainer" {
		t.Errorf("header %v", h)
	}
	if got := LilyPondLicense(ly); got != "Public Domain" {
		t.Errorf("licence %q", got)
	}
	if got := LilyPondPickup(ly); got != "8*2" {
		t.Errorf("pickup %q", got)
	}
	old := []byte(`  copyright = "Public Domain"`)
	if got := LilyPondLicense(old); got != "Public Domain" {
		t.Errorf("old-style licence %q", got)
	}
	for in, want := range map[string]string{
		"Public Domain": "PD", "Creative Commons Attribution 4.0": "CC-BY-4.0",
		"Creative Commons Attribution-ShareAlike 3.0": "", "Creative Commons Attribution-NonCommercial 4.0": "",
	} {
		if got := LicenseFromLilyPond(in); got != want {
			t.Errorf("LicenseFromLilyPond(%q) = %q, want %q", in, got, want)
		}
	}
}

// Over the upstream file verbatim, so it compares directly with what refs scores verify re-fetches.
func TestGate3_CommittedBytesMatchCatalogue(t *testing.T) {
	for _, s := range cat(t).Scores {
		got, err := SumFor(s.ID)
		if err != nil {
			t.Errorf("score %q: %v", s.ID, err)
			continue
		}
		if got != s.SHA256 {
			t.Errorf("score %q: committed bytes hash %s, catalogue says %s "+
				"(re-run scripts/fetch_scores.sh, or the file was edited — scores are committed VERBATIM)",
				s.ID, got, s.SHA256)
		}
	}
}

// TestGate3_ProvenanceIsNamed requires a real upstream location.
func TestGate3_ProvenanceIsNamed(t *testing.T) {
	for _, s := range cat(t).Scores {
		if !strings.HasPrefix(s.Source.URL, "https://") {
			t.Errorf("score %q: source url %q is not an https URL", s.ID, s.Source.URL)
		}
		if !strings.Contains(s.Source.URL, s.Source.Repo) {
			t.Errorf("score %q: url %q does not name repo %q", s.ID, s.Source.URL, s.Source.Repo)
		}
		if strings.TrimSpace(s.Source.Attribution) == "" {
			t.Errorf("score %q: attribution must not be empty", s.ID)
		}
	}
}

// A corrupt or unexpected container fails here rather than inside gen-showcase.
func TestEveryScoreParses(t *testing.T) {
	for _, s := range cat(t).Scores {
		raw, err := Load(s.ID)
		if err != nil {
			t.Errorf("score %q: Load: %v", s.ID, err)
			continue
		}
		if s.FormatOrDefault() == FormatMXL && !musicxml.IsCompressed(raw) {
			t.Errorf("score %q: expected a compressed .mxl container", s.ID)
		}
		sc, err := ParseScore(s.ID)
		if err != nil {
			t.Errorf("score %q: Parse: %v", s.ID, err)
			continue
		}
		if len(sc.Parts) == 0 {
			t.Errorf("score %q: no parts", s.ID)
		}
		if sc.Divisions <= 0 {
			t.Errorf("score %q: divisions=%d", s.ID, sc.Divisions)
		}
		notes := 0
		for _, p := range sc.Parts {
			notes += len(p.Notes)
			// Every percussion note must name its kit piece through a GM key,
			// or the generator cannot route it to an instrument.
			for _, n := range p.Notes {
				if n.Unpitched && n.PercKey == 0 {
					t.Errorf("score %q part %q: unpitched note at div %d has no GM key", s.ID, p.Name, n.OnsetDiv)
					break
				}
			}
		}
		if notes == 0 {
			t.Errorf("score %q: no notes", s.ID)
		}
	}
}

// Keeps the ~5 MB corpus out of shipped binaries: only cmd/gen-showcase, this package and tests may import it.
func TestNoProductionImports(t *testing.T) {
	const self = "github.com/ingyamilmolinar/beatmo/internal/scoreref"
	allowedDirs := map[string]bool{
		"cmd/gen-showcase":  true,
		"cmd/refs":          true,
		"internal/scoreref": true,
	}

	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	srcRoot := filepath.Dir(root) // src/go

	fset := token.NewFileSet()
	err = filepath.Walk(srcRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, rerr := filepath.Rel(srcRoot, path)
		if rerr != nil {
			return nil
		}
		if allowedDirs[filepath.Dir(rel)] {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if perr != nil {
			return nil
		}
		for _, imp := range f.Imports {
			if strings.Trim(imp.Path.Value, `"`) == self {
				t.Errorf("%s imports internal/scoreref — the embedded score corpus "+
					"(~5 MB) must not reach a shipped binary. Consume the generated "+
					"templates in internal/assets instead.", rel)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
}

// A song stays protected until its LAST author's term ends, so wherever the file names a lyricist the catalogue must
// declare (and match) one; a file crediting "Composer" is checked against the composer's dates.
func TestLyricistDeclaredWhereTheScoreNamesOne(t *testing.T) {
	declared := 0
	for _, s := range cat(t).Scores {
		if s.FormatOrDefault() == FormatMIDI {
			if ly, err := LoadCompanion(s.ID); err == nil {
				if poet := LilyPondHeader(ly)["poet"]; poet != "" && s.Work.Lyricist == "" {
					t.Errorf("score %q: LilyPond source names poet %q but the catalogue declares no lyricist", s.ID, poet)
				}
			}
			continue
		}
		raw, err := Load(s.ID)
		if err != nil {
			t.Fatal(err)
		}
		sc, err := musicxml.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if sc.Lyricist == "" {
			continue
		}
		declared++
		if s.Work.Lyricist == "" || s.Work.LyricistDeathYear <= 0 {
			t.Errorf("score %q: file names lyricist %q but the catalogue declares none — Gate 1 cannot check an author it does not know",
				s.ID, sc.Lyricist)
			continue
		}
		if sc.Lyricist == "Composer" {
			if s.Work.LyricistDeathYear != s.Work.ComposerDeathYear {
				t.Errorf("score %q: the composer wrote the words, so the lyricist's death year must be the composer's", s.ID)
			}
			continue
		}
		if s.Work.Lyricist != sc.Lyricist {
			t.Errorf("score %q: catalogue lyricist %q, file says %q", s.ID, s.Work.Lyricist, sc.Lyricist)
		}
	}
	if declared == 0 {
		t.Fatal("no score names a lyricist — this guard proves nothing")
	}
}
