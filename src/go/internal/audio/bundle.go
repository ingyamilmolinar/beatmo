package audio

import (
	"slices"

	"github.com/ingyamilmolinar/beatmo/internal/enum"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/objects"
)

// bundle.go — the audio bundle (spec 2026-10-31 §2.1, §2.5–2.6): every setting
// on top of an instrument's sound — the Sampler edit, the EQ and the insert
// chain. It is a VALUE over the stores live edits already write (sampleEdits,
// the insert-chain manager, the EQ settings), read by CurrentAudioBundle and
// written by ApplyAudioBundle; there is no second store. A nil / empty part is
// absent.
type AudioBundle struct {
	SampleEdit *SampleEdit  `json:"sample_edit,omitempty"`
	EQ         *EQSettings  `json:"eq,omitempty"`
	Effects    []EffectSlot `json:"effects,omitempty"`
}

// Normalized drops parts that do nothing (a flat EQ, an empty chain).
func (b AudioBundle) Normalized() AudioBundle {
	if b.EQ.IsFlat() {
		b.EQ = nil
	}
	if len(b.Effects) == 0 {
		b.Effects = nil
	}
	return b
}

// IsNeutral: the bundle sets nothing.
func (b AudioBundle) IsNeutral() bool {
	n := b.Normalized()
	return n.SampleEdit == nil && n.EQ == nil && n.Effects == nil
}

// Digest is the bundle's version id (objects.Digest of its normalized form).
func (b AudioBundle) Digest() string { return objects.Digest(b.Normalized()) }

// CurrentAudioBundle reads id's bundle from the stores.
func CurrentAudioBundle(id instrument.ID) AudioBundle {
	var b AudioBundle
	if e, ok := SampleEditFor(id); ok {
		b.SampleEdit = &e
	}
	b.EQ = EQSettingsOf(id).Clone()
	b.Effects = GetInsertEffects(id)
	return b.Normalized()
}

// ApplyAudioBundle writes b into id's stores and channel (import, undo, a
// pick): an absent part clears what the store held.
func ApplyAudioBundle(id instrument.ID, b AudioBundle) {
	b = b.Normalized()
	if b.SampleEdit != nil {
		SetSampleEdit(id, *b.SampleEdit)
	} else if _, ok := SampleEditFor(id); ok {
		ClearSampleEdit(id)
	}
	eq := EQSettingsOf(id)
	src := b.EQ
	if src == nil {
		src = NewEQSettings()
	}
	*eq = *src.Clone()
	ApplyEQSettings(id)
	if !slices.EqualFunc(GetInsertEffects(id), b.Effects, effectSlotEqual) {
		SetInsertEffects(id, b.Effects)
	}
}

func effectSlotEqual(a, b EffectSlot) bool {
	return objects.Digest(a) == objects.Digest(b)
}

// BundleMode is how a circuit's bundle for an instrument combines with the
// instrument's own (spec §2.5).
type BundleMode int

const (
	ModeInstrument BundleMode = iota // the instrument's; the circuit bundle is ignored
	ModeCircuit                      // the circuit's; the instrument's is ignored
	ModeOverride                     // part by part: the circuit's where it has one
	ModeMerge                        // FX chained (instrument, then circuit); EQ and Sampler edit as override
)

var bundleModeNames = enum.Names[BundleMode]{Kind: "bundle mode", Names: []string{"instrument", "circuit", "override", "merge"}}

func (m BundleMode) String() string                { return enum.String(bundleModeNames, m) }
func (m BundleMode) MarshalText() ([]byte, error)  { return enum.Marshal(bundleModeNames, m) }
func (m *BundleMode) UnmarshalText(b []byte) error { return enum.Unmarshal(bundleModeNames, b, m) }

// Effective is the bundle an instrument plays with in a circuit: the one
// place the instrument's and the circuit's bundles combine.
func Effective(inst, circ AudioBundle, m BundleMode) AudioBundle {
	inst, circ = inst.Normalized(), circ.Normalized()
	switch m {
	case ModeCircuit:
		return circ
	case ModeOverride, ModeMerge:
		out := inst
		if circ.SampleEdit != nil {
			out.SampleEdit = circ.SampleEdit
		}
		if circ.EQ != nil {
			out.EQ = circ.EQ
		}
		if m == ModeMerge {
			out.Effects = append(slices.Clone(inst.Effects), circ.Effects...)
		} else if circ.Effects != nil {
			out.Effects = circ.Effects
		}
		return out.Normalized()
	default:
		return inst
	}
}
