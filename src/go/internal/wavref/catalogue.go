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

	// Inside Root(); empty when Status is set. The audio itself is NEVER in the repository, only this entry.
	File string `json:"file,omitempty"`

	Status string `json:"status,omitempty"`
	Note   string `json:"note,omitempty"`

	Source  *Source  `json:"source,omitempty"`
	Convert *Convert `json:"convert,omitempty"`

	// The DECODED PCM, not the container: ffmpeg builds vary the metadata, not the audio.
	PCMSHA256 string `json:"pcm_sha256,omitempty"`

	Measured     *Measured     `json:"measured,omitempty"`
	Alternatives []Alternative `json:"alternatives,omitempty"`
}

// So refs fetch can re-create it. Reference audio never ships, so there is no licence to track.
type Source struct {
	Provider string `json:"provider"` // uiowa-mis | vcsl | freesound | archive-org | imslp | local
	URL      string `json:"url"`
}

// The CORPUS rate, not the engine's: the committed calibrations were measured with recording and voice both at it, and
// several analyses count samples, so the same sound rendered at another rate reads as a slightly different one.
const Rate = 44100

// Convert is the deterministic conversion applied after download.
type Convert struct {
	Channels int     `json:"channels"`
	Rate     int     `json:"rate"`
	Bits     int     `json:"bits"`
	StartSec float64 `json:"start_sec,omitempty"`
	MaxSec   float64 `json:"max_sec"`
}

// FROM THE AUDIO, never a filename or upstream metadata (wrong repeatedly).
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
