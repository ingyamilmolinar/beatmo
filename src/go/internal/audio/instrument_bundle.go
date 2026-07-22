package audio

import "fmt"

// Portable, config-only instrument bundle (`*.beatmo-inst.json`). Holds 1+
// instruments plus a NORMALIZED effects library the instruments reference by id
// — effects are defined once here, never welded into an instrument. WAV-based
// instruments carry a reference (name + content hash + version), never PCM.
//
// This is the shareable file format; it is deliberately decoupled from the
// user/game preferences store. See
// docs/superpowers/specs/2026-07-19-portable-instrument-format-design.md.
const (
	// InstrumentBundleFormat is the magic string that identifies our file and
	// lets import reject foreign JSON.
	InstrumentBundleFormat = "beatmo.instrument"
	// InstrumentBundleVersion is OUR integer format version. Bump it only for a
	// breaking change; additive omitempty fields need no bump.
	InstrumentBundleVersion = 1
)

// WAV source kinds — where a referenced WAV can be resolved from. Extensible:
// an unknown kind is skipped by resolution, never fatal, so future kinds don't
// break older readers.
const (
	WavSourceFile         = "file"         // local filesystem path (desktop)
	WavSourceLocalStorage = "localstorage" // browser localStorage / IndexedDB key
	WavSourceURL          = "url"          // remote URL (either platform)
)

// WavSource is one candidate location an instrument's WAV can be resolved from.
// Multiple sources let a shared instrument carry several ways to find the same
// bytes (e.g. a local path AND a URL); resolution tries them in order and
// verifies the Sha256 of whatever it finds.
type WavSource struct {
	Kind string `json:"kind"` // WavSourceFile | WavSourceLocalStorage | WavSourceURL
	Ref  string `json:"ref"`  // path | storage key | URL
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

// InstrumentDoc fully describes ONE shareable instrument (config only). It
// reconstructs the sound on a machine that has never seen it: a synth from its
// recipe, a re-voiced sample from recipe+sample_edit, a user WAV from wav_ref
// (resolved against the recipient's own file/registry).
type InstrumentDoc struct {
	Name        string             `json:"name"`
	Kind        string             `json:"kind"` // "synth" | "sample" | "wav"
	Color       string             `json:"color,omitempty"`
	Recipe      RecipeDoc          `json:"recipe"`
	SynthParams map[string]float64 `json:"synth_params,omitempty"`
	SampleEdit  map[string]float64 `json:"sample_edit,omitempty"`
	WavRef      *WavRef            `json:"wav_ref,omitempty"`
	EffectChain []string           `json:"effect_chain,omitempty"`
	EQ          *InstrumentEQ      `json:"eq,omitempty"`
}

// InstrumentBundle is the top-level file. A single-instrument export writes a
// 1-entry Instruments slice; a "pack" is the same schema with N entries. The
// Effects library is shared across all instruments in the bundle.
type InstrumentBundle struct {
	Format      string          `json:"format"`
	Version     int             `json:"version"`
	Effects     []EffectDef     `json:"effects,omitempty"`
	Instruments []InstrumentDoc `json:"instruments"`
}

// ValidateBundleHeader rejects foreign JSON (wrong/absent magic) and files a
// newer app wrote (version above what we understand). Older versions are
// accepted and migrated forward by the caller.
func ValidateBundleHeader(b *InstrumentBundle) error {
	if b == nil {
		return fmt.Errorf("instrument bundle: nil")
	}
	if b.Format != InstrumentBundleFormat {
		return fmt.Errorf("instrument bundle: unrecognized format %q", b.Format)
	}
	if b.Version < 1 || b.Version > InstrumentBundleVersion {
		return fmt.Errorf("instrument bundle: unsupported version %d (max %d)", b.Version, InstrumentBundleVersion)
	}
	return nil
}
