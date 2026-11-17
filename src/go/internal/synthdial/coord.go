package synthdial

import "math"

// Range is a param's [Min, Max] as the caller's schema declares it.
type Range struct {
	Min, Max float64
}

// Span is a term's absolute travel: its param's value at dial 0 (From) and at
// dial 100 (To). To < From means the param falls as the dial rises.
type Span struct{ From, To float64 }

// SpanFor resolves a term's span against its param's range on one
// instrument: an omitted end defaults to the range's (magnitude: 0 → the
// larger edge), and a declared end beyond the range clips to it — ranges
// differ per instrument family (a bass's decay rate tops out at 12, a
// conga's does not), and a dial reads against the knob the user sees.
func SpanFor(t Term, r Range) Span {
	lo, hi := r.Min, r.Max
	if t.space() == SpaceMagnitude {
		lo, hi = 0, math.Max(r.Max, -r.Min)
	}
	s := Span{From: lo, To: hi}
	if t.From != nil {
		s.From = clamp(*t.From, lo, hi)
	}
	if t.To != nil {
		s.To = clamp(*t.To, lo, hi)
	}
	return s
}

// axis maps a value onto the term's space. In log space a value at or below
// zero (an instant attack, a decay rate of 0 = no decay, off) has no log: it
// floors to the span's lower end. A positive value below the span stays
// outside it like any other out-of-span value.
func (t LiveTerm) axis(v float64) float64 {
	switch t.space() {
	case SpaceLog:
		if v <= 0 {
			v = math.Min(t.Span.From, t.Span.To)
		}
		return math.Log(v)
	case SpaceMagnitude:
		return math.Abs(v)
	}
	return v
}

// Coord is v's position along the term's span: 0 at From, 1 at To, unclamped
// (< 0 before From, > 1 past To — a raw knob edit can leave a value there).
func (t LiveTerm) Coord(v float64) float64 {
	a, b := t.axis(t.Span.From), t.axis(t.Span.To)
	if a == b {
		return 0
	}
	return (t.axis(v) - a) / (b - a)
}

// Value is the param value at span coordinate u, exact at 0 (From) and 1
// (To), clamped to the param's range. ref is the param's current value: a
// magnitude term keeps its sign.
func (t LiveTerm) Value(u, ref float64) float64 {
	var v float64
	switch u {
	case 0:
		v = t.Span.From
	case 1:
		v = t.Span.To
	default:
		a, b := t.axis(t.Span.From), t.axis(t.Span.To)
		v = a + u*(b-a)
		if t.space() == SpaceLog {
			v = math.Exp(v)
		}
	}
	if t.space() == SpaceMagnitude && ref < 0 {
		v = -v
	}
	return clamp(v, t.Range.Min, t.Range.Max)
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
