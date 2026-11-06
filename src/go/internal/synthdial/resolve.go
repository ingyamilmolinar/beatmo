package synthdial

// LiveTerm is a term that is live on one instrument: its param's range and
// the absolute span it travels there.
type LiveTerm struct {
	Term
	Range Range
	Span  Span
}

// Resolved is a dial as it applies to one instrument: only its live terms.
type Resolved struct {
	Dial  Dial
	Terms []LiveTerm
}

// Live reports whether a term is live on the instrument (declared, visible,
// audible) and its param's range. Supplied by the caller — the package never
// sees a schema.
type Live func(t Term) (Range, bool)

// Resolve returns the dials on one card for one instrument, in file order:
// each keeps only its live terms, a dial with none is dropped, and at most
// MaxDialsPerCard survive.
func Resolve(f File, card string, live Live) []Resolved {
	var out []Resolved
	for _, d := range f.Dials {
		if len(out) == MaxDialsPerCard {
			break
		}
		if !d.OnCard(card) {
			continue
		}
		var terms []LiveTerm
		for _, t := range d.Terms {
			if r, ok := live(t); ok {
				terms = append(terms, LiveTerm{Term: t, Range: r, Span: SpanFor(t, r)})
			}
		}
		if len(terms) == 0 {
			continue
		}
		out = append(out, Resolved{Dial: d, Terms: terms})
	}
	return out
}
