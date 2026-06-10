//go:build !test && !js

package audio

/*
#cgo CFLAGS: -I${SRCDIR}/../../../c
#cgo LDFLAGS: -L${SRCDIR}/../../../../build -ldrums -lm
#include <stdlib.h>
#include "adsr.h"
#include "wavetable.h"
#include "pan.h"
#include "effects.h"

// Inline-accessor trampolines: pan_set/pan_init/adsr_tick are static inline
// in their headers; tiny named wrappers give cgo a stable symbol to call.
static void beatmo_pan_set(pan_t *p, float pan) { pan_set(p, pan); }
static void beatmo_pan_init(pan_t *p)           { pan_init(p); }
static float beatmo_adsr_tick(adsr_t *env)      { return adsr_tick(env); }
*/
import "C"
import "unsafe"

// Thin CGo wrappers for the standalone DSP utility modules (adsr.c,
// wavetable.c, pan.c). The synth engines reach these units through their own
// C-side calls (modular.c drives wt_osc_tick, fmsynth.c reads generated
// tables), so until now no Go symbol pulled their full APIs into the native
// build. These wrappers make the units directly drivable from Go — used by
// dsp_units_native_test.go and available to future Go callers (e.g. a
// Go-side stereo pan stage).
//
// Memory discipline: every struct that C retains a pointer to (wavetable_t,
// its table buffer, wt_osc_t) lives in C-malloc'd memory so no Go pointer is
// ever stored across a cgo call (cgo pointer-passing rules). pan_t and
// adsr_t are only written during the call that receives them, so Go-held
// values are fine there.

// ── ADSR ────────────────────────────────────────────────────────────────────

type cADSR struct {
	env C.adsr_t
}

func newCADSR(sr int, attack, decay, sustain, release float64, exponential bool) *cADSR {
	e := &cADSR{}
	exp := 0
	if exponential {
		exp = 1
	}
	C.adsr_init(&e.env, C.int(sr),
		C.float(attack), C.float(decay), C.float(sustain), C.float(release),
		C.int(exp))
	return e
}

func (e *cADSR) Trigger()       { C.adsr_trigger(&e.env) }
func (e *cADSR) Release()       { C.adsr_release(&e.env) }
func (e *cADSR) Reset()         { C.adsr_reset(&e.env) }
func (e *cADSR) Tick() float32  { return float32(C.beatmo_adsr_tick(&e.env)) }
func (e *cADSR) Value() float32 { return float32(e.env.value) }
func (e *cADSR) Stage() int     { return int(e.env.stage) }

func (e *cADSR) Process(out []float32) {
	if len(out) == 0 {
		return
	}
	C.adsr_process(&e.env, (*C.float)(unsafe.Pointer(&out[0])), C.int(len(out)))
}

// ── Wavetable ───────────────────────────────────────────────────────────────

// cWavetable owns a C-malloc'd wavetable_t plus its table buffer; Free must
// be called (tests defer it).
type cWavetable struct {
	wt  *C.wavetable_t
	buf *C.float
}

type wtShape int

const (
	wtSine wtShape = iota
	wtSaw
	wtSquare
	wtTriangle
)

func newCWavetable(shape wtShape, length, harmonics int) *cWavetable {
	w := &cWavetable{}
	w.wt = (*C.wavetable_t)(C.malloc(C.size_t(unsafe.Sizeof(C.wavetable_t{}))))
	// +1 for the guard point table[length] == table[0].
	w.buf = (*C.float)(C.malloc(C.size_t(length+1) * C.size_t(unsafe.Sizeof(C.float(0)))))
	switch shape {
	case wtSaw:
		C.wt_generate_saw(w.wt, w.buf, C.int(length), C.int(harmonics))
	case wtSquare:
		C.wt_generate_square(w.wt, w.buf, C.int(length), C.int(harmonics))
	case wtTriangle:
		C.wt_generate_triangle(w.wt, w.buf, C.int(length), C.int(harmonics))
	default:
		C.wt_generate_sine(w.wt, w.buf, C.int(length))
	}
	return w
}

func (w *cWavetable) Free() {
	C.free(unsafe.Pointer(w.buf))
	C.free(unsafe.Pointer(w.wt))
}

func (w *cWavetable) Length() int { return int(w.wt.length) }

// At returns table[i] (i may be length: the guard point).
func (w *cWavetable) At(i int) float32 {
	tbl := unsafe.Slice((*float32)(unsafe.Pointer(w.wt.table)), int(w.wt.length)+1)
	return tbl[i]
}

// cWtOsc owns a C-malloc'd wt_osc_t (the struct retains the wavetable
// pointer across calls, so it must live in C memory).
type cWtOsc struct {
	osc *C.wt_osc_t
}

func newCWtOsc(w *cWavetable, freq float64, sampleRate int) *cWtOsc {
	o := &cWtOsc{}
	o.osc = (*C.wt_osc_t)(C.malloc(C.size_t(unsafe.Sizeof(C.wt_osc_t{}))))
	C.wt_osc_init(o.osc, w.wt, C.double(freq), C.int(sampleRate))
	return o
}

func (o *cWtOsc) Free() { C.free(unsafe.Pointer(o.osc)) }

func (o *cWtOsc) SetFreq(freq float64, sampleRate int) {
	C.wt_osc_set_freq(o.osc, C.double(freq), C.int(sampleRate))
}

func (o *cWtOsc) PhaseInc() float64 { return float64(o.osc.phase_inc) }

func (o *cWtOsc) Process(out []float32) {
	if len(out) == 0 {
		return
	}
	C.wt_osc_process(o.osc, (*C.float)(unsafe.Pointer(&out[0])), C.int(len(out)))
}

func (o *cWtOsc) ProcessFM(freqMod, out []float32, sampleRate int) {
	if len(out) == 0 || len(freqMod) < len(out) {
		return
	}
	C.wt_osc_process_fm(o.osc,
		(*C.float)(unsafe.Pointer(&freqMod[0])),
		(*C.float)(unsafe.Pointer(&out[0])),
		C.int(len(out)), C.int(sampleRate))
}

// ── Send-delay (direct C construction) ──────────────────────────────────────

// cSendDelay drives effects.c's delay_t directly — the production path
// (send_effects.go) always initializes with the default 3 kHz damping, so
// delay_init's dampingHz<=0 "no filtering" branch is only reachable here.
// The struct and its line buffer live in C memory (delay_t retains the
// buffer pointer).
type cSendDelay struct {
	d   *C.delay_t
	buf *C.float
	n   int
}

func newCSendDelay(delaySamples int, feedback, dampingHz float64, sampleRate int) *cSendDelay {
	sd := &cSendDelay{n: delaySamples}
	sd.d = (*C.delay_t)(C.malloc(C.size_t(unsafe.Sizeof(C.delay_t{}))))
	sd.buf = (*C.float)(C.malloc(C.size_t(delaySamples) * C.size_t(unsafe.Sizeof(C.float(0)))))
	C.delay_init(sd.d, sd.buf, C.int(delaySamples),
		C.float(feedback), C.float(dampingHz), C.int(sampleRate))
	return sd
}

func (sd *cSendDelay) Free() {
	C.free(unsafe.Pointer(sd.buf))
	C.free(unsafe.Pointer(sd.d))
}

func (sd *cSendDelay) LPCoeff() float32 { return float32(sd.d.lpCoeff) }

// Process runs the delay in place over buf.
func (sd *cSendDelay) Process(buf []float32) {
	if len(buf) == 0 {
		return
	}
	C.delay_process(sd.d, (*C.float)(unsafe.Pointer(&buf[0])), C.int(len(buf)))
}

// ── Pan ─────────────────────────────────────────────────────────────────────

type cPan struct {
	p C.pan_t
}

func newCPan() *cPan {
	p := &cPan{}
	C.beatmo_pan_init(&p.p)
	return p
}

// Set applies the equal-power pan law. pan is -1 (left) .. +1 (right).
func (p *cPan) Set(pan float64) { C.beatmo_pan_set(&p.p, C.float(pan)) }

func (p *cPan) Gains() (l, r float32) {
	return float32(p.p.gain_l), float32(p.p.gain_r)
}

// ProcessMonoToLR accumulates the panned mono input into left/right.
func (p *cPan) ProcessMonoToLR(in, left, right []float32) {
	n := len(in)
	if n == 0 || len(left) < n || len(right) < n {
		return
	}
	C.pan_process_mono_to_lr(&p.p,
		(*C.float)(unsafe.Pointer(&in[0])),
		(*C.float)(unsafe.Pointer(&left[0])),
		(*C.float)(unsafe.Pointer(&right[0])),
		C.int(n))
}

// ProcessMonoToInterleaved writes panned LR pairs; out needs len(in)*2.
func (p *cPan) ProcessMonoToInterleaved(in, out []float32) {
	n := len(in)
	if n == 0 || len(out) < n*2 {
		return
	}
	C.pan_process_mono_to_interleaved(&p.p,
		(*C.float)(unsafe.Pointer(&in[0])),
		(*C.float)(unsafe.Pointer(&out[0])),
		C.int(n))
}
