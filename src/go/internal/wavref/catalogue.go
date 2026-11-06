package wavref

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/schema"
)

// catalogueFormat names this document in schema.Validate errors.
const catalogueFormat = "beatmo.wavref-catalogue"

// Tier values.
const (
	TierInstrument = "instrument" // one isolated note/phrase
	TierSong       = "song"       // a full recording of a template's piece
)

// Status values. An empty status means the ref has real audio.
const (
	StatusSynthetic   = "synthetic"    // no acoustic counterpart exists; counts as covered
	StatusNeedsSource = "needs-source" // acoustic, but no isolated recording adopted yet
)

//go:embed catalogue.json
var catalogueJSON []byte

// Cat is the reference catalogue.
type Cat struct {
	schema.Envelope
	Refs []Ref `json:"refs"`
}

// Ref is one reference entry.
type Ref struct {
	ID   string `json:"id"`
	Tier string `json:"tier"`

	// Instruments lists every instrument id this reference is ground truth
	// for. One recording legitimately serves an instrument and its
	// character variants (violin and violin-ensemble share a sample).
	Instruments []instrument.ID `json:"instruments,omitempty"`
	// Template is the template stem a song-tier reference belongs to.
	Template string `json:"template,omitempty"`

	// File is the bare filename inside the reference-audio root (Root()).
	// Empty when Status is set. The audio itself is NEVER in the repository:
	// only this catalogue entry is (see TestNoAudioFilesInRepository).
	File string `json:"file,omitempty"`

	Status string `json:"status,omitempty"`
	Note   string `json:"note,omitempty"`

	Source  *Source  `json:"source,omitempty"`
	Convert *Convert `json:"convert,omitempty"`

	// PCMSHA256 hashes the DECODED PCM (samples + rate), not the container
	// bytes. Container metadata varies between ffmpeg builds; the audio
	// does not, so this is what makes verification meaningful.
	PCMSHA256 string `json:"pcm_sha256,omitempty"`

	Measured     *Measured     `json:"measured,omitempty"`
	Alternatives []Alternative `json:"alternatives,omitempty"`
}

// Source records where the audio came from, so `refs fetch` can re-create
// it. Reference audio is local dev tooling that never ships, so there is no
// licence to track.
type Source struct {
	Provider string `json:"provider"` // uiowa-mis | vcsl | freesound | archive-org | imslp | local
	URL      string `json:"url"`
}

// Rate is the sample rate every reference recording is converted to, and the
// rate the reference tools render the synth at to compare a voice with one.
// It is the CORPUS's rate, not the engine's (48 kHz since 2026-10-27): the
// committed calibrations (thresholds, organic range, match hubness) were
// measured with recording and voice both at this rate, and several analyses
// count samples, so the same sound rendered at another rate reads as a
// slightly different one (refs match lost conga and kalimba that way).
const Rate = 44100

// Convert is the deterministic conversion applied after download.
type Convert struct {
	Channels int     `json:"channels"`
	Rate     int     `json:"rate"`
	Bits     int     `json:"bits"`
	StartSec float64 `json:"start_sec,omitempty"`
	MaxSec   float64 `json:"max_sec"`
}

// Measured holds values detected FROM THE AUDIO, never parsed from a
// filename or upstream metadata (which has been wrong repeatedly).
type Measured struct {
	F0Hz        float64 `json:"f0_hz"`
	SemisFromA3 int     `json:"semis_from_a3"`
	Peak        float64 `json:"peak"`
	RMS         float64 `json:"rms"`
	DurationSec float64 `json:"duration_sec"`
}

// Alternative is a candidate reference that lost the ear comparison.
type Alternative struct {
	URL    string `json:"url"`
	Reason string `json:"reason"`
}

var (
	catOnce sync.Once
	catVal  Cat
	catErr  error
)

// Catalogue parses the embedded catalogue once and returns it.
func Catalogue() (Cat, error) {
	catOnce.Do(func() {
		if err := json.Unmarshal(catalogueJSON, &catVal); err != nil {
			catErr = fmt.Errorf("wavref: parse catalogue.json: %w", err)
			return
		}
		catErr = catVal.Envelope.Validate(catalogueFormat)
	})
	return catVal, catErr
}

// Ref returns the entry with the given id.
func (c Cat) Ref(id string) (Ref, bool) {
	for _, r := range c.Refs {
		if r.ID == id {
			return r, true
		}
	}
	return Ref{}, false
}

// ForInstrument returns every entry that is ground truth for inst.
func (c Cat) ForInstrument(inst instrument.ID) []Ref {
	var out []Ref
	for _, r := range c.Refs {
		for _, i := range r.Instruments {
			if i == inst {
				out = append(out, r)
				break
			}
		}
	}
	return out
}

// ForTemplate returns the song reference for a template stem.
func (c Cat) ForTemplate(stem string) (Ref, bool) {
	for _, r := range c.Refs {
		if r.Tier == TierSong && r.Template == stem {
			return r, true
		}
	}
	return Ref{}, false
}

// Validate checks the structural invariants the tests and CLI rely on.
func (c Cat) Validate() error {
	seen := map[string]bool{}
	for _, r := range c.Refs {
		if r.ID == "" {
			return fmt.Errorf("wavref: ref with empty id")
		}
		if seen[r.ID] {
			return fmt.Errorf("wavref: duplicate ref id %q", r.ID)
		}
		seen[r.ID] = true

		switch r.Tier {
		case TierInstrument:
			if len(r.Instruments) == 0 {
				return fmt.Errorf("wavref: %s: instrument-tier ref lists no instruments", r.ID)
			}
		case TierSong:
			if r.Template == "" {
				return fmt.Errorf("wavref: %s: song-tier ref names no template", r.ID)
			}
		default:
			return fmt.Errorf("wavref: %s: unknown tier %q", r.ID, r.Tier)
		}

		switch r.Status {
		case "":
			if r.File == "" {
				return fmt.Errorf("wavref: %s: real ref has no file", r.ID)
			}
			if r.Source == nil {
				return fmt.Errorf("wavref: %s: real ref has no source", r.ID)
			}
		case StatusSynthetic, StatusNeedsSource:
			if r.File != "" || r.PCMSHA256 != "" {
				return fmt.Errorf("wavref: %s: status ref must not carry file or hash", r.ID)
			}
			if r.Note == "" {
				return fmt.Errorf("wavref: %s: status ref must explain itself", r.ID)
			}
		default:
			return fmt.Errorf("wavref: %s: unknown status %q", r.ID, r.Status)
		}
	}
	return nil
}
