package fingerprint

import "fmt"

// descriptor.go — one row of the measurement registry.
//
// The registry is the single source of truth for every measurement the
// analysis battery makes. Before it, cmd/refs/diagnose.go hand-wrote nine
// axes as inline mkAxis calls with their thresholds scattered as
// package-level constants; that shape does not survive growing to ~150
// descriptors, and it offers nowhere to hang the orthogonality, robustness
// and calibration analyses that make the battery trustworthy.
//
// Declaring a descriptor is meant to be one literal with no code. Anything
// exotic supplies a Compare override.

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

// Repr says WHICH REPRESENTATION of the signal a descriptor reads. Grouping
// the report by Repr answers "which domain does this mismatch actually live
// in" — a question the flat axis list could not express.
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
	// ID is the stable machine key ("noise.fraction", "seg.attack.centroid").
	// Goldens and the calibration tables key on it, so it must not change
	// once committed.
	ID string

	// Group and Name are the human-facing labels. They are load-bearing:
	// cmd/refs/diagnose_acceptance_test.go keys on them, so the nine
	// migrated axes must keep their original strings byte-for-byte.
	Group string
	Name  string

	Scope Scope
	Repr  Repr
	Kind  Kind

	// Drift and Mismatch override Kind's default thresholds for this
	// descriptor. Both zero means "use the Kind defaults". They exist because
	// some kinds have no single natural scale: KindRate covers Hz/sec, dB/sec
	// and units/sec^2, which sit orders of magnitude apart, so the scale has
	// to come from the descriptor rather than from its kind.
	Drift, Mismatch float64

	// Unit overrides the rendered unit suffix. Empty means the Kind's default
	// ("Hz" for KindHz, "s" for KindSeconds, none otherwise). A rate
	// descriptor needs it: Hz/sec and Hz are not the same quantity and must
	// not print the same.
	Unit string

	// Compound marks a measurement whose vector components are NOT peers:
	// index 0 is the headline value and the rest are flags or fallback
	// quantities its comparator reads (env.ring_t60 carries an
	// "extrapolated" bit and a decay rate alongside the T60).
	//
	// It exists because harnesses that perturb a measurement have to know
	// which components may be moved, and the only available proxy was
	// "Compare != nil" — which was wrong the moment a genuine all-peer vector
	// (mode.ratios, decay.per_mode) acquired a custom comparator, and quietly
	// weakened the scale gate for those rows to a single-component nudge that
	// a mean-based reduction dilutes to nothing.
	Compound bool

	// Demoted marks a descriptor that is measured and displayed but excluded
	// from every "worst offending axes" ranking and from validate's trusted
	// set. Set when calibration showed the axis misleads on this corpus.
	Demoted bool

	// Doc must explain what the descriptor reads AND why that source rather
	// than a neighbouring one. The registry discipline test enforces
	// non-emptiness; reviewers enforce the rest.
	Doc string

	// Applies gates the descriptor on measured signal character — pitched vs
	// unpitched, sustained vs percussive — never on an instrument→family
	// table, which would go stale as instruments change. Required: use
	// AlwaysApplies to put "applies everywhere" on the record explicitly
	// rather than leaving it nil by omission.
	Applies func(*Bundle) bool

	// Extract reads the measurement from an already-computed Bundle.
	Extract func(*Bundle) Measurement

	// Compare overrides the default comparator for this descriptor. Nil means
	// CompareByKind(d.Kind, …).
	//
	// It is HANDED this descriptor's effective (drift, mismatch) — the same
	// pair Thresholds() returns, calibration included — and must use them
	// rather than literals. A comparator that hardcoded its thresholds was
	// unreachable from the calibration file: `refs calibrate-thresholds`
	// could write an entry for it, the registry would apply it, and the
	// comparator would go on using the number in its own source, so the axis
	// stayed saturated while the report claimed it had been calibrated
	// (mode.ratios fired on 91% of instruments against a hand-guessed 80
	// cents, with a good-group p90 an order of magnitude larger).
	//
	// A comparator MAY transform the pair into its own working domain
	// (band.t60 declares ratios and compares mean|log2|); when it does it
	// must still report Delta.Mag in the DECLARED units, so calibration and
	// the comparator agree on what a threshold means.
	Compare func(ref, synth Measurement, drift, mismatch float64) Delta
}

// CompareWith applies this descriptor's comparator, falling back to the
// Kind-appropriate default.
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

// Thresholds returns this descriptor's effective (drift, mismatch) pair: its
// own overrides when set, otherwise its Kind's defaults.
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

// AlwaysApplies is the explicit no-op predicate. Descriptors must set Applies
// rather than leaving it nil, so "this applies everywhere" is a decision on
// the record rather than an omission nobody reviewed.
func AlwaysApplies(*Bundle) bool { return true }

// RequiresPitched gates descriptors whose meaning depends on a harmonic
// series existing at all — every partial, harmonic-ladder and inharmonicity
// measurement. Gated on the MEASURED signal so it self-classifies.
//
// F0Hz > 0 is NOT sufficient and was the original bug: DetectF0's octave
// guard returns a number for any signal, so a hi-hat reported 56.5 Hz and
// every harmonic descriptor happily measured against a fundamental that does
// not exist. The confidence gate is what makes this predicate mean "a
// harmonic series is actually there".
func RequiresPitched(b *Bundle) bool {
	fp := b.Fingerprint()
	return fp.F0Hz > 0 && fp.F0Confidence >= F0ReliableConfidence
}

// naUnreliableF0 explains a suppressed harmonic measurement in terms a reader
// can act on: it names the number that was rejected and the confidence that
// rejected it, so "not measurable" reads as a finding about the signal rather
// than as a tooling gap.
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

// RequiresSustain gates descriptors defined only over a sustain plateau
// (sustain modulation, sustain-segment timbre). A percussive hit has no
// plateau, and reporting 0 there would read as "perfectly steady" when the
// truth is "not applicable".
func RequiresSustain(b *Bundle) bool {
	s := b.Segments()
	return !s.Silent && !s.NoSustainPlateau && s.SustainSec > 0
}

// RequiresPercussive gates the kick/snare-family descriptors: an audible
// signal that peaks and decays without ever establishing a plateau.
func RequiresPercussive(b *Bundle) bool {
	s := b.Segments()
	return !s.Silent && s.NoSustainPlateau
}

// RequiresAudible gates on the signal not being silent — the weakest useful
// precondition, for descriptors that work on any real signal.
func RequiresAudible(b *Bundle) bool { return !b.Segments().Silent }
