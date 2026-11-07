package audio

import (
	"fmt"

	"github.com/ingyamilmolinar/beatmo/internal/enum"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/schema"
)

// The shareable `*.beatmo-inst.json`: config only, decoupled from prefs. Effects live once in a normalized library
// the instruments reference by id; a WAV instrument carries a reference (name + hash + version), never PCM.

// Lets import reject foreign JSON. "Which format", orthogonal to schema_version's "which revision".
const InstrumentBundleFormat = "beatmo.instrument"

// A new kind is a new schema version, so an unknown spelling is a decode error, never a silently skipped source.
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

// Derived from what the entry carries (KindOf) and checked on import.
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

// Sources are tried in order; whatever is found is verified against Sha256.
type WavSource struct {
	Kind WavSourceKind `json:"kind"`
	Ref  string        `json:"ref"` // path | storage key | URL
}

// The bytes live in storage the user manages, not in the bundle. Sha256 is the portable truth; Version is a local label.
type WavRef struct {
	Name    string      `json:"name"`
	Sha256  string      `json:"sha256"`
	Version int         `json:"version,omitempty"`
	Sources []WavSource `json:"sources,omitempty"`
}

// Dedup identity is (Type, Params, Enabled).
type EffectDef struct {
	ID      string             `json:"id"`
	Name    string             `json:"name,omitempty"`
	Type    EffectType         `json:"type"`
	Params  map[string]float64 `json:"params,omitempty"`
	Enabled bool               `json:"enabled"`
}

// Inline: unlike insert effects it is a fixed shape, not a reusable unit.
type InstrumentEQ struct {
	GainsDB     []float64    `json:"gains_db,omitempty"`
	BandsHz     [][2]float64 `json:"bands_hz,omitempty"`
	BandMuted   []bool       `json:"band_muted,omitempty"`
	HPFEnabled  bool         `json:"hpf_enabled,omitempty"`
	HPFCutoffHz float64      `json:"hpf_cutoff_hz,omitempty"`
	LPFEnabled  bool         `json:"lpf_enabled,omitempty"`
	LPFCutoffHz float64      `json:"lpf_cutoff_hz,omitempty"`
}

// Enough to rebuild the sound on a machine that never saw it: a synth or re-voiced sample from its document (+
// sample_edit), a user WAV from wav_ref resolved against the recipient's own files.
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

// A pack is the same schema with N entries; the effects library is shared across them.
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

// Every writer goes through this, so no path can emit an unstamped bundle.
func NewInstrumentBundle() InstrumentBundle {
	return InstrumentBundle{Envelope: schema.Current(), Format: InstrumentBundleFormat}
}

// An absent schema_version is rejected too.
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
