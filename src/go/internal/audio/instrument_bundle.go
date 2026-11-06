package audio

import (
	"fmt"

	"github.com/ingyamilmolinar/beatmo/internal/enum"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/schema"
)

// Portable, config-only instrument bundle (`*.beatmo-inst.json`). Holds 1+
// instruments plus a NORMALIZED effects library the instruments reference by id
// — effects are defined once here, never welded into an instrument. WAV-based
// instruments carry a reference (name + content hash + version), never PCM.
//
// This is the shareable file format; it is deliberately decoupled from the
// user/game preferences store. See
// docs/superpowers/specs/2026-07-19-portable-instrument-format-design.md.

// InstrumentBundleFormat is the magic string that identifies our file and
// lets import reject foreign JSON. It answers "which format is this?" and is
// orthogonal to schema_version, which answers "which revision of it?".
const InstrumentBundleFormat = "beatmo.instrument"

// WavSourceKind is where a referenced WAV can be resolved from. A closed
// enum: a new kind is a new schema version, so an unknown spelling is a
// decode error, never a silently skipped source.
type WavSourceKind int

const (
	WavSourceFile         WavSourceKind = iota // local filesystem path (desktop)
	WavSourceLocalStorage                      // browser localStorage / IndexedDB key
	WavSourceURL                               // remote URL (either platform)
)

var wavSourceKindNames = enum.Names[WavSourceKind]{Kind: "WAV source kind", Names: []string{"file", "localstorage", "url"}}

func (k WavSourceKind) String() string               { return enum.String(wavSourceKindNames, k) }
func (k WavSourceKind) MarshalText() ([]byte, error) { return enum.Marshal(wavSourceKindNames, k) }
func (k *WavSourceKind) UnmarshalText(b []byte) error {
	return enum.Unmarshal(wavSourceKindNames, b, k)
}

// BundleKind says how a bundled instrument is made. It is derived from what
// the entry carries (BundleInstrument.KindOf) and checked on import.
type BundleKind int

const (
	BundleSynth  BundleKind = iota // its document
	BundleSample                   // its document + a sample edit
	BundleWAV                      // a user WAV, by hash-pinned reference
)

var bundleKindNames = enum.Names[BundleKind]{Kind: "bundled instrument kind", Names: []string{"synth", "sample", "wav"}}

func (k BundleKind) String() string                { return enum.String(bundleKindNames, k) }
func (k BundleKind) MarshalText() ([]byte, error)  { return enum.Marshal(bundleKindNames, k) }
func (k *BundleKind) UnmarshalText(b []byte) error { return enum.Unmarshal(bundleKindNames, b, k) }

// WavSource is one candidate location an instrument's WAV can be resolved from.
// Multiple sources let a shared instrument carry several ways to find the same
// bytes (e.g. a local path AND a URL); resolution tries them in order and
// verifies the Sha256 of whatever it finds.
type WavSource struct {
	Kind WavSourceKind `json:"kind"`
	Ref  string        `json:"ref"` // path | storage key | URL
}

// WavRef pins a specific version of a user WAV by content — the audio bytes
// live in separate storage the user manages, NOT in this bundle. Sha256 is the
// portable truth; Version is a friendly label from the local WAV registry;
// Sources are candidate locations (local filesystem, browser storage, or remote
// URL) resolution attempts in order.
type WavRef struct {
	Name    string      `json:"name"`
	Sha256  string      `json:"sha256"`
	Version int         `json:"version,omitempty"`
	Sources []WavSource `json:"sources,omitempty"`
}

// EffectDef is one entry in the bundle's normalized effects library. Instruments
// reference it by ID from their EffectChain. Identity for dedup is (Type,
// Params, Enabled).
type EffectDef struct {
	ID      string             `json:"id"`
	Name    string             `json:"name,omitempty"`
	Type    EffectType         `json:"type"`
	Params  map[string]float64 `json:"params,omitempty"`
	Enabled bool               `json:"enabled"`
}

// InstrumentEQ is the optional inline per-instrument EQ (a fixed 10-band curve +
// HPF/LPF). Unlike insert effects it is not a variable-length, reusable unit, so
// it stays inline on the instrument rather than in the normalized library.
type InstrumentEQ struct {
	GainsDB     []float64    `json:"gains_db,omitempty"`
	BandsHz     [][2]float64 `json:"bands_hz,omitempty"`
	BandMuted   []bool       `json:"band_muted,omitempty"`
	HPFEnabled  bool         `json:"hpf_enabled,omitempty"`
	HPFCutoffHz float64      `json:"hpf_cutoff_hz,omitempty"`
	LPFEnabled  bool         `json:"lpf_enabled,omitempty"`
	LPFCutoffHz float64      `json:"lpf_cutoff_hz,omitempty"`
}

// BundleInstrument fully describes ONE shareable instrument (config only). It
// reconstructs the sound on a machine that has never seen it: a synth from its
// complete instrument document (spec 2026-10-04 §3), a re-voiced sample from
// that document + sample_edit, a user WAV from wav_ref (resolved against the
// recipient's own file/registry).
type BundleInstrument struct {
	Name        string             `json:"name"`
	Kind        BundleKind         `json:"kind"`
	Color       string             `json:"color,omitempty"`
	Synth       *instrument.Doc    `json:"synth,omitempty"`
	SampleEdit  map[string]float64 `json:"sample_edit,omitempty"`
	WavRef      *WavRef            `json:"wav_ref,omitempty"`
	EffectChain []string           `json:"effect_chain,omitempty"`
	EQ          *InstrumentEQ      `json:"eq,omitempty"`
}

// InstrumentBundle is the top-level file. A single-instrument export writes a
// 1-entry Instruments slice; a "pack" is the same schema with N entries. The
// Effects library is shared across all instruments in the bundle.
type InstrumentBundle struct {
	schema.Envelope
	Format      string             `json:"format"`
	Effects     []EffectDef        `json:"effects,omitempty"`
	Instruments []BundleInstrument `json:"instruments"`
}

// KindOf is the kind the entry's own fields imply.
func (b BundleInstrument) KindOf() BundleKind {
	switch {
	case b.WavRef != nil:
		return BundleWAV
	case len(b.SampleEdit) > 0:
		return BundleSample
	}
	return BundleSynth
}

// NewInstrumentBundle returns a bundle stamped with the current schema
// version and format magic. Every writer must go through this rather than
// building a literal, so no path can emit an unstamped bundle.
func NewInstrumentBundle() InstrumentBundle {
	return InstrumentBundle{Envelope: schema.Current(), Format: InstrumentBundleFormat}
}

// ValidateBundleHeader rejects foreign JSON (wrong/absent magic) and any
// schema_version other than the current one, an absent one included.
func ValidateBundleHeader(b *InstrumentBundle) error {
	if b == nil {
		return fmt.Errorf("instrument bundle: nil")
	}
	if b.Format != InstrumentBundleFormat {
		return fmt.Errorf("instrument bundle: unrecognized format %q", b.Format)
	}
	if b.SchemaVersion == "" {
		return fmt.Errorf("instrument bundle: missing %q", schema.Field)
	}
	if err := b.Validate("instrument bundle"); err != nil {
		return err
	}
	for i, inst := range b.Instruments {
		if want := inst.KindOf(); inst.Kind != want {
			return fmt.Errorf("instrument bundle: instruments[%d] %q: kind %q, but the entry carries a %s", i, inst.Name, inst.Kind, want)
		}
	}
	return nil
}
