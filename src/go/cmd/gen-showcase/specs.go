package main

import "fmt"

// showcases builds every template from templateSpecs(). It is deliberately
// mechanical: the musical content comes from the committed scores, so there is
// nothing here to hand-tune.
func showcases() []Showcase {
	out := make([]Showcase, 0, len(templateSpecs()))
	for _, spec := range templateSpecs() {
		sh, _, err := buildFromScore(spec)
		if err != nil {
			panic(fmt.Sprintf("gen-showcase: %v", err))
		}
		out = append(out, sh)
	}
	return out
}
