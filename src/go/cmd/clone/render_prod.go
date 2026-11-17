//go:build !test && !js

package main

import (
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// renderShipped is the sound a trigger plays (document FX, gate, peak
// normalisation × amplitude, percussion resampled) — the only render analysis
// tools may judge (CLAUDE.md: RenderAsShipped).
func renderShipped(d instrument.Doc, rate int, pitch float64) []float64 {
	return audio.RenderAsShipped(d, rate, pitch)
}

const realEngine = true
