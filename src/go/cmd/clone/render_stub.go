//go:build test || js

package main

import (
	"github.com/ingyamilmolinar/beatmo/internal/audio/clone"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// renderShipped under -tags test: the C engine is a stub there, so the CLI's
// plumbing runs on the document's mode model, summed in Go and peak-normalised.
func renderShipped(d instrument.Doc, rate int, pitch float64) []float64 {
	x := clone.Synthesize(clone.DocModes(d, pitch, rate), rate, int(d.Render.Seconds*float64(rate)))
	p := 0.0
	for _, v := range x {
		p = max(p, v, -v)
	}
	for i := range x {
		if p > 0 {
			x[i] *= d.Render.Amplitude / p
		}
	}
	return x
}

const realEngine = false
