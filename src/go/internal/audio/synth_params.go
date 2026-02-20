//go:build !test && !js

package audio

/*
#cgo CFLAGS: -I${SRCDIR}/../../../c
#cgo LDFLAGS: -L${SRCDIR}/../../../../build -ldrums -lm
#include "drums.h"
#include <stdlib.h>
*/
import "C"
import "unsafe"

// SynthParams mirrors the C synth_params struct for per-node instrument
// customization. All zero values produce default instrument behavior.
type SynthParams struct {
	Pitch      float64 // Semitone offset from default (0 = default)
	Decay      float64 // Decay multiplier (1.0 = default, 0.5 = half, 2.0 = double)
	Tone       float64 // Tone/brightness: -1 = dark, 0 = default, 1 = bright
	Attack     float64 // Attack multiplier (1.0 = default, smaller = sharper)
	Drive      float64 // Saturation amount (0 = none, 1 = heavy)
	Body       float64 // Body/resonance emphasis (0 = default, 1 = max)
	Color      float64 // Timbral character shift: -1..1 (instrument-specific)
	Brightness float64 // High-frequency content: 0 = default, 1 = max shimmer
}

// IsDefault returns true if all params are at their default values.
func (p SynthParams) IsDefault() bool {
	return p.Pitch == 0 && p.Decay == 0 && p.Tone == 0 &&
		p.Attack == 0 && p.Drive == 0 && p.Body == 0 &&
		p.Color == 0 && p.Brightness == 0
}

// toCParams converts Go SynthParams to a C synth_params pointer.
// Returns nil if params are default (so C functions use their hardcoded defaults).
func (p SynthParams) toCParams() *C.synth_params {
	if p.IsDefault() {
		return nil
	}
	cp := &C.synth_params{
		pitch:      C.float(p.Pitch),
		decay:      C.float(p.Decay),
		tone:       C.float(p.Tone),
		attack:     C.float(p.Attack),
		drive:      C.float(p.Drive),
		body:       C.float(p.Body),
		color:      C.float(p.Color),
		brightness: C.float(p.Brightness),
	}
	return cp
}

// cParamRenderer is a render function that accepts synth_params.
type cParamRenderer func(buf []float32, sampleRate, samples int, params SynthParams)

// Parameterized render wrappers.

func renderSnareP(buf []float32, sampleRate, samples int, params SynthParams) {
	if samples > len(buf) || len(buf) == 0 || samples == 0 {
		return
	}
	C.render_snare_p((*C.float)(unsafe.Pointer(&buf[0])), C.int(sampleRate), C.int(samples), params.toCParams())
}

func renderKickP(buf []float32, sampleRate, samples int, params SynthParams) {
	if samples > len(buf) || len(buf) == 0 || samples == 0 {
		return
	}
	C.render_kick_p((*C.float)(unsafe.Pointer(&buf[0])), C.int(sampleRate), C.int(samples), params.toCParams())
}

func renderHiHatP(buf []float32, sampleRate, samples int, params SynthParams) {
	if samples > len(buf) || len(buf) == 0 || samples == 0 {
		return
	}
	C.render_hihat_p((*C.float)(unsafe.Pointer(&buf[0])), C.int(sampleRate), C.int(samples), params.toCParams())
}

func renderClapP(buf []float32, sampleRate, samples int, params SynthParams) {
	if samples > len(buf) || len(buf) == 0 || samples == 0 {
		return
	}
	C.render_clap_p((*C.float)(unsafe.Pointer(&buf[0])), C.int(sampleRate), C.int(samples), params.toCParams())
}

func renderTomP(buf []float32, sampleRate, samples int, params SynthParams) {
	if samples > len(buf) || len(buf) == 0 || samples == 0 {
		return
	}
	C.render_tom_p((*C.float)(unsafe.Pointer(&buf[0])), C.int(sampleRate), C.int(samples), params.toCParams())
}

func renderCowbellP(buf []float32, sampleRate, samples int, params SynthParams) {
	if samples > len(buf) || len(buf) == 0 || samples == 0 {
		return
	}
	C.render_cowbell_p((*C.float)(unsafe.Pointer(&buf[0])), C.int(sampleRate), C.int(samples), params.toCParams())
}

// ParamRenderFuncs maps instrument base names to their _p() C render wrappers.
var ParamRenderFuncs = map[string]cParamRenderer{
	"snare":   renderSnareP,
	"kick":    renderKickP,
	"hihat":   renderHiHatP,
	"clap":    renderClapP,
	"tom":     renderTomP,
	"cowbell": renderCowbellP,
}
