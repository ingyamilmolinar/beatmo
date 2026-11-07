// Package scoreref catalogues the scores the templates are generated from. Unlike audio (never committed), every score is
// CC0 or similar and committed, so generation is offline and reproducible. Admission gates: a public-domain WORK, a
// permissive ENCODING, verifiable PROVENANCE (docs/score-sources.md). ~5 MB: never imported by a shipped binary.
package scoreref

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/musicxml"
	"github.com/ingyamilmolinar/beatmo/internal/schema"
	"github.com/ingyamilmolinar/beatmo/internal/smf"
)

//go:embed catalogue.json
var catalogueJSON []byte

//go:embed scores
var scoreFS embed.FS

// Format identifies this document type, orthogonal to schema.Version which
// identifies the revision.
const Format = "beatmo.scoreref-catalogue"

// CC-BY-4.0 is satisfiable by the committed notice, so its entries MUST carry an Attribution. NonCommercial and
// ShareAlike must never be added: NC forbids a shipping product, SA would push its terms onto the generated templates.
var AllowedLicenses = []string{"CC0-1.0", "PD", "unrestricted", "CC-BY-4.0"}

// US: published in year Y enters the public domain on 1 January Y+96. EU: an author dying in Y releases on 1 January
// Y+71. Clearing both at once needs no per-piece jurisdiction analysis.
const (
	usPublicationTermYears = 96
	euLifePlusYears        = 71
)

// PublicationCutoff is the latest first-publication year that is public domain
// on the given date.
func PublicationCutoff(now time.Time) int { return now.Year() - usPublicationTermYears }

// DeathCutoff is the latest author death year whose works are public domain on
// the given date.
func DeathCutoff(now time.Time) int { return now.Year() - euLifePlusYears }

// OriginalComposer is the composer credited on Beatmo's own CC0 pieces.
const OriginalComposer = "Beatmo"

// Score formats. An empty Format means FormatMXL, the corpus default.
const (
	FormatMXL      = "mxl"
	FormatMusicXML = "musicxml"
	FormatMIDI     = "midi"
)

// Providers with special provenance rules.
const (
	// ProviderMutopia is the Mutopia Project: LilyPond editions with MIDI.
	ProviderMutopia = "mutopia"
	// ProviderBeatmo marks a score Beatmo authored itself (rhythmic idioms).
	// The committed file is the original; there is no upstream to re-fetch.
	ProviderBeatmo = "beatmo"
)

// Work is the composition — facts about the music, not about the encoding.
type Work struct {
	Title                string `json:"title"`
	Composer             string `json:"composer"`
	Catalog              string `json:"catalog"`
	FirstPublicationYear int    `json:"first_publication_year"`
	ComposerDeathYear    int    `json:"composer_death_year"`

	// Arranger / Lyricist are further authors whose term must also have run
	// out. A co-written song stays protected until its LAST author's term ends
	// (EU Directive 2011/77), even when only the melody is used.
	Arranger          string `json:"arranger,omitempty"`
	ArrangerDeathYear int    `json:"arranger_death_year,omitempty"`
	Lyricist          string `json:"lyricist,omitempty"`
	LyricistDeathYear int    `json:"lyricist_death_year,omitempty"`

	// Traditional marks a work with no author: a folk tune or a rhythmic idiom
	// (son clave, the habanera cell). It is admitted on Reference — a citation
	// establishing that the work is traditional — rather than on dates.
	Traditional bool   `json:"traditional,omitempty"`
	Reference   string `json:"reference,omitempty"`

	// A DRUM PATTERN encoded from a published transcription of a record's groove: a short rhythm is generally not
	// protectable, so it is admitted on its Reference and only Beatmo's own CC0 encoding is committed.
	Rhythm bool `json:"rhythm,omitempty"`

	// Original marks a piece Beatmo composed and dedicated to the public
	// domain (CC0). It has no term to wait out: it is admitted on its
	// dedication, which TestBeatmoScoresAreSelfDescribing checks is Beatmo's.
	Original bool `json:"original,omitempty"`
}

// Source is the encoding: where it came from and under what terms.
type Source struct {
	Provider string `json:"provider"`
	Repo     string `json:"repo"`
	URL      string `json:"url"`
	License  string `json:"license"`
	// Attribution is the credit line reproduced in NOTICE-scores.md.
	Attribution string `json:"attribution"`
	// Verbatim; a test checks it corroborates License, so the catalogue cannot claim a licence the file contradicts.
	EmbeddedRights string `json:"embedded_rights"`
	// SourceEdition is the printed edition the transcriber worked from, when
	// the file names one.
	SourceEdition string `json:"source_edition"`
}

// Score is one catalogue entry.
type Score struct {
	ID     string `json:"id"`
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
	// Format is FormatMXL (default), FormatMusicXML or FormatMIDI.
	Format string `json:"format,omitempty"`
	// The anacrusis as a LilyPond duration, checked against the companion's \partial: MIDI cannot express it, and without
	// it every downbeat moves.
	Pickup string `json:"pickup,omitempty"`
	// Companion is the source a derived file was built from — a MIDI edition's
	// LilyPond file — committed beside it so its licence can be checked.
	Companion *Companion `json:"companion,omitempty"`
	Templates []string   `json:"templates"`
	Work      Work       `json:"work"`
	Source    Source     `json:"source"`
}

// Companion is a committed source file that corroborates a score.
type Companion struct {
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
	URL    string `json:"url"`
}

// IsGroove reports whether the score is a percussion groove — a rhythmic
// idiom or a famous break, encoded by Beatmo — which backs a template's GROOVE
// layer. A template is backed by at most one melody score and one groove.
func (s Score) IsGroove() bool {
	return s.Source.Provider == ProviderBeatmo && !s.Work.Original && strings.HasPrefix(s.ID, "groove-")
}

// FormatOrDefault returns the score's format, defaulting to FormatMXL.
func (s Score) FormatOrDefault() string {
	if s.Format == "" {
		return FormatMXL
	}
	return s.Format
}

// Cat is the catalogue document.
type Cat struct {
	schema.Envelope
	Format string  `json:"format"`
	Scores []Score `json:"scores"`
}

var (
	catOnce sync.Once
	catVal  Cat
	catErr  error
)

// Catalogue returns the parsed catalogue. The embedded document is immutable,
// so it is decoded once. Callers treat the result as read-only.
func Catalogue() (Cat, error) {
	catOnce.Do(func() {
		if err := json.Unmarshal(catalogueJSON, &catVal); err != nil {
			catErr = fmt.Errorf("scoreref: catalogue.json: %w", err)
			return
		}
		if err := catVal.Envelope.Validate(Format); err != nil {
			catErr = fmt.Errorf("scoreref: catalogue.json: %w", err)
			return
		}
		if catVal.Format != Format {
			catErr = fmt.Errorf("scoreref: catalogue.json: format %q, want %q", catVal.Format, Format)
		}
	})
	return catVal, catErr
}

// Embedded and test-validated, so a failure is a build defect.
func MustCatalogue() Cat {
	c, err := Catalogue()
	if err != nil {
		panic(err.Error())
	}
	return c
}

// Ref returns the catalogue entry for id.
func Ref(id string) (Score, bool) {
	c, err := Catalogue()
	if err != nil {
		return Score{}, false
	}
	for _, s := range c.Scores {
		if s.ID == id {
			return s, true
		}
	}
	return Score{}, false
}

// IDs returns every score id, sorted.
func IDs() []string {
	c, err := Catalogue()
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(c.Scores))
	for _, s := range c.Scores {
		out = append(out, s.ID)
	}
	sort.Strings(out)
	return out
}

// The upstream file verbatim (a .mxl container), so the catalogue SHA256 compares directly with the upstream artifact.
func Load(id string) ([]byte, error) {
	s, ok := Ref(id)
	if !ok {
		return nil, fmt.Errorf("scoreref: unknown score %q", id)
	}
	b, err := scoreFS.ReadFile("scores/" + s.File)
	if err != nil {
		return nil, fmt.Errorf("scoreref: read %s: %w", s.File, err)
	}
	return b, nil
}

// MustLoad is Load for generator code, where an absent score is a build defect.
func MustLoad(id string) []byte {
	b, err := Load(id)
	if err != nil {
		panic(err.Error())
	}
	return b
}

// Files returns the names of every committed score file, sorted. Used by the
// orphan check.
func Files() ([]string, error) {
	ents, err := scoreFS.ReadDir("scores")
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(ents))
	for _, e := range ents {
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out, nil
}

// SumFor returns the hex SHA256 of the committed bytes for id.
func SumFor(id string) (string, error) {
	b, err := Load(id)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// WorkIsPublicDomain reports whether the WORK (not the encoding) clears Gate 1
// on the given date.
func (w Work) WorkIsPublicDomain(now time.Time) bool {
	return w.Gate1Failure(now) == ""
}

// Gate1Failure explains why the work fails Gate 1 on the given date, or
// returns "" when it passes.
func (w Work) Gate1Failure(now time.Time) string {
	pub, death := PublicationCutoff(now), DeathCutoff(now)
	switch {
	case w.Original:
		if w.Composer != OriginalComposer {
			return fmt.Sprintf("an original work must be Beatmo's own (composer %q, want %q)", w.Composer, OriginalComposer)
		}
		return ""
	case w.Rhythm:
		if strings.TrimSpace(w.Reference) == "" {
			return "a rhythm pattern must cite the transcription it was encoded from"
		}
		return ""
	}
	if w.Traditional {
		if strings.TrimSpace(w.Reference) == "" {
			return "a traditional work must cite a reference establishing it"
		}
		if w.FirstPublicationYear > pub {
			return fmt.Sprintf("edition first published %d, after the %d cut-off", w.FirstPublicationYear, pub)
		}
	} else {
		if w.FirstPublicationYear <= 0 || w.FirstPublicationYear > pub {
			return fmt.Sprintf("first_publication_year=%d must be >0 and <=%d", w.FirstPublicationYear, pub)
		}
		if w.ComposerDeathYear <= 0 || w.ComposerDeathYear > death {
			return fmt.Sprintf("composer_death_year=%d must be >0 and <=%d", w.ComposerDeathYear, death)
		}
	}
	if w.Arranger != "" && (w.ArrangerDeathYear <= 0 || w.ArrangerDeathYear > death) {
		return fmt.Sprintf("arranger %s: arranger_death_year=%d must be >0 and <=%d", w.Arranger, w.ArrangerDeathYear, death)
	}
	if w.Lyricist != "" && (w.LyricistDeathYear <= 0 || w.LyricistDeathYear > death) {
		return fmt.Sprintf("lyricist %s: lyricist_death_year=%d must be >0 and <=%d", w.Lyricist, w.LyricistDeathYear, death)
	}
	return ""
}

// LicenseAllowed reports whether lic is on the Gate 2 allowlist.
func LicenseAllowed(lic string) bool {
	for _, a := range AllowedLicenses {
		if a == lic {
			return true
		}
	}
	return false
}

// LoadCompanion returns the raw bytes of a score's companion source file.
func LoadCompanion(id string) ([]byte, error) {
	s, ok := Ref(id)
	if !ok {
		return nil, fmt.Errorf("scoreref: unknown score %q", id)
	}
	if s.Companion == nil {
		return nil, fmt.Errorf("scoreref: score %q has no companion file", id)
	}
	b, err := scoreFS.ReadFile("scores/" + s.Companion.File)
	if err != nil {
		return nil, fmt.Errorf("scoreref: read %s: %w", s.Companion.File, err)
	}
	return b, nil
}

// ParseScore loads and parses a score in whatever format it is committed in,
// returning the one score model the template generator consumes.
func ParseScore(id string) (musicxml.Score, error) {
	s, ok := Ref(id)
	if !ok {
		return musicxml.Score{}, fmt.Errorf("scoreref: unknown score %q", id)
	}
	raw, err := Load(id)
	if err != nil {
		return musicxml.Score{}, err
	}
	switch s.FormatOrDefault() {
	case FormatMXL, FormatMusicXML:
		return musicxml.Parse(raw)
	case FormatMIDI:
		div, err := smfDivision(raw)
		if err != nil {
			return musicxml.Score{}, fmt.Errorf("scoreref: %s: %w", id, err)
		}
		q, err := LilyPondDurationQuarters(s.Pickup)
		if err != nil {
			return musicxml.Score{}, fmt.Errorf("scoreref: %s: pickup: %w", id, err)
		}
		return smf.Parse(raw, smf.Options{PickupTicks: int(q * float64(div))})
	}
	return musicxml.Score{}, fmt.Errorf("scoreref: %s: unknown format %q", id, s.Format)
}

// smfDivision reads the ticks-per-quarter of a Standard MIDI File header.
func smfDivision(raw []byte) (int, error) {
	if len(raw) < 14 || string(raw[:4]) != "MThd" {
		return 0, fmt.Errorf("not a Standard MIDI File")
	}
	return int(raw[12])<<8 | int(raw[13]), nil
}
