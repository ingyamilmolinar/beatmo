package fingerprint

import "fmt"

// One row of the measurement registry, the single source of every measurement: declaring a descriptor is one
// literal with no code, and anything exotic supplies a Compare override.

// Scope says WHICH PART of the signal a descriptor measures.
type Scope int

const (
	ScopeWhole         Scope = iota // the entire signal
	ScopeSegment                    // one envelope segment (attack, sustain, …)
	ScopeBand                       // one frequency band
	ScopeTrack                      // a statistic over a frame-wise track
	ScopeAblationStage              // a synthesis stage, measured by ablation
	ScopeVoiceTap                   // a C-level tap inside the voice render
)

// String renders s for reports and test messages.
func (s Scope) String() string {
	switch s {
	case ScopeSegment:
		return "segment"
	case ScopeBand:
		return "band"
	case ScopeTrack:
		return "track"
	case ScopeAblationStage:
		return "ablation-stage"
	case ScopeVoiceTap:
		return "voice-tap"
	default:
		return "whole"
	}
}

// Grouping by Repr answers which domain a mismatch actually lives in.
type Repr int

const (
	ReprTime       Repr = iota // raw samples / amplitude envelope
	ReprSpectrum               // linear-frequency magnitude spectrum
	ReprLogFreq                // constant-Q / log-frequency
	ReprMel                    // mel filterbank
	ReprCepstral               // cepstrum / MFCC
	ReprModulation             // modulation spectrum of a frame-wise track
)

// String renders r for reports and test messages.
func (r Repr) String() string {
	switch r {
	case ReprSpectrum:
		return "spectrum"
	case ReprLogFreq:
		return "log-freq"
	case ReprMel:
		return "mel"
	case ReprCepstral:
		return "cepstral"
	case ReprModulation:
		return "modulation"
	default:
		return "time"
	}
}

// Descriptor declares one measurement.
type Descriptor struct {
	// Goldens and calibration tables key on it: it must not change once committed.
	ID string

	// The diagnose acceptance test keys on these strings byte for byte.
	Group string
	Name  string

	Scope Scope
	Repr  Repr
	Kind  Kind

	// Both zero = the Kind defaults. Some kinds have no single scale (KindRate spans Hz/s, dB/s, units/s²), so the scale
	// comes from the descriptor.
	Drift, Mismatch float64

	// Empty = the Kind's default; Hz/s and Hz must not print the same.
	Unit string

	// Index 0 is the headline value and the rest are flags or fallbacks its comparator reads, so perturbation harnesses
	// know which components may move ("Compare != nil" was the wrong proxy).
	Compound bool

	// Measured and shown, but out of every ranking and validate's trusted set: calibration showed it misleads.
	Demoted bool

	// What it reads AND why that source rather than a neighbour's.
	Doc string

	// Gated on measured signal character, never an instrument→family table that would go stale. Required: AlwaysApplies
	// puts "everywhere" on the record.
	Applies func(*Bundle) bool

	// Extract reads the measurement from an already-computed Bundle.
	Extract func(*Bundle) Measurement

	// nil = CompareByKind. It is HANDED the effective (drift, mismatch), calibration included, and must use them: a
	// hardcoded threshold is unreachable from the calibration file and stays saturated. A comparator may transform them,
	// but must report Delta.Mag in the DECLARED units.
	Compare func(ref, synth Measurement, drift, mismatch float64) Delta
}

func (d Descriptor) CompareWith(ref, synth Measurement) Delta {
	if d.Compare != nil {
		drift, mismatch := d.Thresholds()
		return d.Compare(ref, synth, drift, mismatch)
	}
	if d.Drift > 0 || d.Mismatch > 0 {
		drift, mismatch := d.Thresholds()
		return CompareWithThresholds(d.Kind, ref, synth, drift, mismatch)
	}
	return CompareByKind(d.Kind, ref, synth)
}

func (d Descriptor) Thresholds() (drift, mismatch float64) {
	kd, km := d.Kind.Thresholds()
	if d.Drift > 0 {
		kd = d.Drift
	}
	if d.Mismatch > 0 {
		km = d.Mismatch
	}
	return kd, km
}

// --- Applicability predicates ------------------------------------------------

// So "applies everywhere" is a recorded decision, not an omission nobody reviewed.
func AlwaysApplies(*Bundle) bool { return true }

// For descriptors that need a harmonic series. F0Hz > 0 is NOT enough: DetectF0 returns a number for any signal (a
// hi-hat read 56.5 Hz), so the confidence gate is what makes this mean a series exists.
func RequiresPitched(b *Bundle) bool {
	fp := b.Fingerprint()
	return fp.F0Hz > 0 && fp.F0Confidence >= F0ReliableConfidence
}

// Names the rejected number and the confidence that rejected it, so "not measurable" reads as a finding about the
// signal.
func naUnreliableF0(b *Bundle) string {
	fp := b.Fingerprint()
	if fp.F0Hz <= 0 {
		return "no fundamental detected"
	}
	return fmt.Sprintf(
		"no reliable fundamental: F0 detection returned %.1f Hz at confidence %.2f (below %.2f), "+
			"so the harmonic-series assumption this measurement rests on does not hold",
		fp.F0Hz, fp.F0Confidence, F0ReliableConfidence)
}

// A percussive hit has no plateau, and 0 there would read as "perfectly steady" rather than "not applicable".
func RequiresSustain(b *Bundle) bool {
	s := b.Segments()
	return !s.Silent && !s.NoSustainPlateau && s.SustainSec > 0
}

// Peaks and decays without ever establishing a plateau.
func RequiresPercussive(b *Bundle) bool {
	s := b.Segments()
	return !s.Silent && s.NoSustainPlateau
}

func RequiresAudible(b *Bundle) bool { return !b.Segments().Silent }
