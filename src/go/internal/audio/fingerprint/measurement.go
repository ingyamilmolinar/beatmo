package fingerprint

import (
	"fmt"
	"math"
)

// One float64 cannot carry a harmonic ladder, an MFCC vector, a band profile or a curve; Measurement lets every
// measurement, scalar or not, travel through the same API with a verdict and a threshold.

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

// OK=false is an HONEST not-applicable, never a misleading zero: a silent clip, a hit with no plateau or an unpitched
// source reports WHY it could not be measured.
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

// labels, when present, are parallel to vs so the report can name each component.
func Vector(vs []float64, labels []string) Measurement {
	return Measurement{Form: FormVector, Values: vs, Labels: labels, OK: true}
}

func Track(vs []float64, hopSec float64) Measurement {
	return Measurement{Form: FormTrack, Values: vs, HopSec: hopSec, OK: true}
}

// Every numeric field stays zero, so a caller that forgets OK cannot read a value never measured.
func Unavailable(reason string) Measurement {
	return Measurement{NA: reason}
}

// The scalar, or component 0; a compound measurement therefore puts its headline value first.
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

// Values[0] is right for a COMPOUND measurement and misleading for an all-peer vector, where component 0 is just the
// first band or mode (band.t60 printed its -1 sentinel as "-1.000 s", mode.ratios a constant 0 cents). An all-peer
// vector says what it is and how much was measurable; the comparator's Display carries the compared value.
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

// Unfittable bands carry a NEGATIVE sentinel to stay aligned with octaveBandLabels, so negative or NaN is "not
// measured", never a value.
func countMeasurableBands(vs []float64) int {
	n := 0
	for _, v := range vs {
		if v > 0 && !math.IsNaN(v) {
			n++
		}
	}
	return n
}
