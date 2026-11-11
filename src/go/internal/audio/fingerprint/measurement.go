package fingerprint

import (
	"fmt"
	"math"
)

// measurement.go — the generic result of one descriptor extraction.
//
// A single float64 cannot carry the harmonic ladder H1-H16, the MFCC vector,
// a band-energy profile, the partial-ratio list, or an envelope curve. Before
// the registry, cmd/refs/diagnose.go worked around that by smuggling partial
// ratios out through dedicated RefPartialRatios/SynthPartialRatios fields,
// entirely outside the axis system — so they had no verdict, no threshold and
// no place in any comparison. Measurement is the shape that lets every
// measurement, scalar or not, travel through the same API.

// Form tags which of Measurement's payloads is populated.
type Form int

const (
	FormScalar Form = iota // one number
	FormVector             // a fixed profile: H1..H16, MFCC, band energies
	FormTrack              // a time series at HopSec resolution
)

// String renders f for test failure messages.
func (f Form) String() string {
	switch f {
	case FormVector:
		return "vector"
	case FormTrack:
		return "track"
	default:
		return "scalar"
	}
}

// Measurement is one descriptor's reading of one wave.
//
// OK=false is an HONEST not-applicable, never a misleading zero. It
// generalizes the discipline envelope_segments.go already established with
// its Silent / NoSustainPlateau / RingT60Extrapolated flags: a silent clip, a
// percussive hit with no sustain plateau, or an unpitched source with no
// reliable F0 reports WHY it could not be measured rather than reporting a
// zero that reads as a measurement.
type Measurement struct {
	Form   Form
	Scalar float64
	Values []float64 // vector payload, or track samples
	Labels []string  // vector only; parallel to Values when present
	HopSec float64   // track only: seconds between consecutive Values
	OK     bool
	NA     string // why, when !OK
}

// Scalar builds an available scalar measurement.
func Scalar(v float64) Measurement {
	return Measurement{Form: FormScalar, Scalar: v, OK: true}
}

// Vector builds an available vector measurement. labels may be nil; when
// present it must be parallel to vs so the report can name each component.
func Vector(vs []float64, labels []string) Measurement {
	return Measurement{Form: FormVector, Values: vs, Labels: labels, OK: true}
}

// Track builds an available time-series measurement sampled every hopSec
// seconds.
func Track(vs []float64, hopSec float64) Measurement {
	return Measurement{Form: FormTrack, Values: vs, HopSec: hopSec, OK: true}
}

// Unavailable builds a not-applicable measurement carrying the reason every
// numeric field stays zero, so a caller that forgets to check OK cannot read
// a value that was never measured.
func Unavailable(reason string) Measurement {
	return Measurement{NA: reason}
}

// Primary returns the single number that best represents this measurement,
// for report columns that show one value per axis: the scalar itself, or the
// first component of a vector/track.
//
// A compound measurement (one whose extra components exist only to feed a
// custom comparator, like ring T60's extrapolation flag and fallback decay
// rate) therefore puts its headline value first, so the report still prints
// the number a reader expects.
func (m Measurement) Primary() float64 {
	if !m.OK {
		return 0
	}
	if m.Form == FormScalar {
		return m.Scalar
	}
	if len(m.Values) == 0 {
		return 0
	}
	return m.Values[0]
}

// Headline renders the REFERENCE/SYNTH column for a measurement whose
// Primary() would MISLEAD, and reports false when Primary() is honest.
//
// Primary() answers "one number for the report" with Values[0], which is
// correct for a COMPOUND measurement (ring T60 deliberately puts its headline
// first and carries flags behind it) and wrong for an all-peer vector, where
// component 0 is just the first band/mode and carries no more authority than
// any other. Three rows made that concretely misleading:
//
//   - band.t60's component 0 is the 125 Hz band, which is very often the
//     UNFITTABLE sentinel -1 — so the column printed "-1.000 s", a number
//     that is not a duration at all, while the verdict came from the mean
//     over the bands that WERE fittable;
//   - mode.ratios' component 0 is the anchor mode's gap to itself, so it
//     printed a constant "0 cents" on every instrument ever measured;
//   - decay.per_mode's component 0 is the lowest mode's rate, which the
//     comparator averages away.
//
// So an all-peer vector says what it is and how much of it was measurable,
// and never offers a component as if it were the reading. The comparator's
// own Display still carries the compared quantity.
func (d Descriptor) Headline(m Measurement) (string, bool) {
	if !m.OK || m.Form == FormScalar || d.Compound {
		return "", false
	}
	if m.Form == FormTrack {
		return fmt.Sprintf("(track: %d frames)", len(m.Values)), true
	}
	if d.Scope == ScopeBand {
		return fmt.Sprintf("(vector: %d/%d bands measurable)",
			countMeasurableBands(m.Values), len(m.Values)), true
	}
	return fmt.Sprintf("(vector: %d components)", len(m.Values)), true
}

// countMeasurableBands counts the components of a per-band vector that carry
// a real reading. Band descriptors mark an unfittable band with a NEGATIVE
// sentinel (bandT60Unfit) rather than dropping it, so the vector stays
// positionally aligned with octaveBandLabels; a negative or NaN component is
// therefore "not measured here", never a value.
func countMeasurableBands(vs []float64) int {
	n := 0
	for _, v := range vs {
		if v > 0 && !math.IsNaN(v) {
			n++
		}
	}
	return n
}
