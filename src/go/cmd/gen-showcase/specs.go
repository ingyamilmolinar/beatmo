package main

import "fmt"

// Mechanical on purpose: the musical content comes from the committed scores.
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
