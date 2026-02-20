//go:build !test && !js

package audio

/*
#cgo CFLAGS: -I${SRCDIR}/../../../c
#cgo LDFLAGS: -L${SRCDIR}/../../../../build -ldrums -lm
#include "insert_fx.h"
#include <stdlib.h>
*/
import "C"
import "unsafe"

// ── Distortion ──────────────────────────────────────────────────────────────

type cDistortion struct {
	d C.ifx_distortion_t
}

func newDistortion(sr int, params map[string]float64) *cDistortion {
	d := &cDistortion{}
	C.ifx_distortion_init(&d.d, C.int(sr),
		C.float(params["drive"]), C.float(params["tone"]), C.float(params["mix"]))
	return d
}

func (d *cDistortion) ProcessSample(x float64) float64 {
	in := [1]C.float{C.float(x)}
	out := [1]C.float{}
	C.ifx_distortion_process(&d.d, &in[0], &out[0], 1)
	return float64(out[0])
}

func (d *cDistortion) ProcessBlockBuf(in []float32, out []float32, samples int) {
	if samples <= 0 {
		return
	}
	C.ifx_distortion_process(&d.d,
		(*C.float)(unsafe.Pointer(&in[0])),
		(*C.float)(unsafe.Pointer(&out[0])),
		C.int(samples))
}

func (d *cDistortion) Reset() { C.ifx_distortion_reset(&d.d) }
func (d *cDistortion) SetParam(name string, v float64) {
	cn := C.CString(name)
	C.ifx_distortion_set_param(&d.d, cn, C.float(v))
	C.free(unsafe.Pointer(cn))
}

// ── Delay ───────────────────────────────────────────────────────────────────

type cDelay struct {
	d      C.ifx_delay_t
	bufPtr unsafe.Pointer // owned C buffer
}

func newDelay(sr int, params map[string]float64) *cDelay {
	d := &cDelay{}
	// Max buffer: 1 second at sample rate.
	bufLen := sr
	if bufLen <= 0 {
		bufLen = 44100
	}
	d.bufPtr = C.malloc(C.size_t(bufLen) * C.size_t(unsafe.Sizeof(C.float(0))))
	C.ifx_delay_init(&d.d, C.int(sr), (*C.float)(d.bufPtr), C.int(bufLen),
		C.float(params["time"]), C.float(params["feedback"]), C.float(params["mix"]))
	return d
}

func (d *cDelay) ProcessSample(x float64) float64 {
	in := [1]C.float{C.float(x)}
	out := [1]C.float{}
	C.ifx_delay_process(&d.d, &in[0], &out[0], 1)
	return float64(out[0])
}

func (d *cDelay) ProcessBlockBuf(in []float32, out []float32, samples int) {
	if samples <= 0 {
		return
	}
	C.ifx_delay_process(&d.d,
		(*C.float)(unsafe.Pointer(&in[0])),
		(*C.float)(unsafe.Pointer(&out[0])),
		C.int(samples))
}

func (d *cDelay) Reset() { C.ifx_delay_reset(&d.d) }
func (d *cDelay) SetParam(name string, v float64) {
	cn := C.CString(name)
	C.ifx_delay_set_param(&d.d, cn, C.float(v))
	C.free(unsafe.Pointer(cn))
}

// ── Reverb ──────────────────────────────────────────────────────────────────

type cReverb struct {
	r      C.ifx_reverb_t
	memPtr unsafe.Pointer // owned C buffer
}

func newReverb(sr int, params map[string]float64) *cReverb {
	r := &cReverb{}
	memSize := int(C.ifx_reverb_mem_size(C.int(sr)))
	r.memPtr = C.malloc(C.size_t(memSize) * C.size_t(unsafe.Sizeof(C.float(0))))
	C.ifx_reverb_init(&r.r, C.int(sr), (*C.float)(r.memPtr),
		C.float(params["room"]), C.float(params["damping"]), C.float(params["mix"]))
	return r
}

func (r *cReverb) ProcessSample(x float64) float64 {
	in := [1]C.float{C.float(x)}
	out := [1]C.float{}
	C.ifx_reverb_process(&r.r, &in[0], &out[0], 1)
	return float64(out[0])
}

func (r *cReverb) ProcessBlockBuf(in []float32, out []float32, samples int) {
	if samples <= 0 {
		return
	}
	C.ifx_reverb_process(&r.r,
		(*C.float)(unsafe.Pointer(&in[0])),
		(*C.float)(unsafe.Pointer(&out[0])),
		C.int(samples))
}

func (r *cReverb) Reset() { C.ifx_reverb_reset(&r.r) }
func (r *cReverb) SetParam(name string, v float64) {
	cn := C.CString(name)
	C.ifx_reverb_set_param(&r.r, cn, C.float(v))
	C.free(unsafe.Pointer(cn))
}

// ── Chorus ──────────────────────────────────────────────────────────────────

type cChorus struct {
	c      C.ifx_chorus_t
	bufPtr unsafe.Pointer // owned C buffer
}

func newChorus(sr int, params map[string]float64) *cChorus {
	c := &cChorus{}
	depthMs := params["depth"]
	if depthMs <= 0 {
		depthMs = 5
	}
	srF := float64(sr)
	if srF <= 0 {
		srF = 44100
	}
	bufLen := int(depthMs*0.001*srF*2) + 64
	c.bufPtr = C.malloc(C.size_t(bufLen) * C.size_t(unsafe.Sizeof(C.float(0))))
	C.ifx_chorus_init(&c.c, C.int(sr), (*C.float)(c.bufPtr), C.int(bufLen),
		C.float(params["rate"]), C.float(params["depth"]), C.float(params["mix"]))
	return c
}

func (c *cChorus) ProcessSample(x float64) float64 {
	in := [1]C.float{C.float(x)}
	out := [1]C.float{}
	C.ifx_chorus_process(&c.c, &in[0], &out[0], 1)
	return float64(out[0])
}

func (c *cChorus) ProcessBlockBuf(in []float32, out []float32, samples int) {
	if samples <= 0 {
		return
	}
	C.ifx_chorus_process(&c.c,
		(*C.float)(unsafe.Pointer(&in[0])),
		(*C.float)(unsafe.Pointer(&out[0])),
		C.int(samples))
}

func (c *cChorus) Reset() { C.ifx_chorus_reset(&c.c) }
func (c *cChorus) SetParam(name string, v float64) {
	cn := C.CString(name)
	C.ifx_chorus_set_param(&c.c, cn, C.float(v))
	C.free(unsafe.Pointer(cn))
}

// ── Bitcrusher ──────────────────────────────────────────────────────────────

type cBitcrusher struct {
	b C.ifx_bitcrusher_t
}

func newBitcrusher(_ int, params map[string]float64) *cBitcrusher {
	b := &cBitcrusher{}
	C.ifx_bitcrusher_init(&b.b,
		C.float(params["bits"]), C.float(params["rate"]), C.float(params["mix"]))
	return b
}

func (b *cBitcrusher) ProcessSample(x float64) float64 {
	in := [1]C.float{C.float(x)}
	out := [1]C.float{}
	C.ifx_bitcrusher_process(&b.b, &in[0], &out[0], 1)
	return float64(out[0])
}

func (b *cBitcrusher) ProcessBlockBuf(in []float32, out []float32, samples int) {
	if samples <= 0 {
		return
	}
	C.ifx_bitcrusher_process(&b.b,
		(*C.float)(unsafe.Pointer(&in[0])),
		(*C.float)(unsafe.Pointer(&out[0])),
		C.int(samples))
}

func (b *cBitcrusher) Reset() { C.ifx_bitcrusher_reset(&b.b) }
func (b *cBitcrusher) SetParam(name string, v float64) {
	cn := C.CString(name)
	C.ifx_bitcrusher_set_param(&b.b, cn, C.float(v))
	C.free(unsafe.Pointer(cn))
}

// ── Filter ──────────────────────────────────────────────────────────────────

type cFilter struct {
	f C.ifx_filter_t
}

func newFilter(sr int, params map[string]float64) *cFilter {
	f := &cFilter{}
	C.ifx_filter_init(&f.f, C.int(sr),
		C.float(params["mode"]), C.float(params["cutoff"]),
		C.float(params["q"]), C.float(params["mix"]))
	return f
}

func (f *cFilter) ProcessSample(x float64) float64 {
	in := [1]C.float{C.float(x)}
	out := [1]C.float{}
	C.ifx_filter_process(&f.f, &in[0], &out[0], 1)
	return float64(out[0])
}

func (f *cFilter) ProcessBlockBuf(in []float32, out []float32, samples int) {
	if samples <= 0 {
		return
	}
	C.ifx_filter_process(&f.f,
		(*C.float)(unsafe.Pointer(&in[0])),
		(*C.float)(unsafe.Pointer(&out[0])),
		C.int(samples))
}

func (f *cFilter) Reset() { C.ifx_filter_reset(&f.f) }
func (f *cFilter) SetParam(name string, v float64) {
	cn := C.CString(name)
	C.ifx_filter_set_param(&f.f, cn, C.float(v))
	C.free(unsafe.Pointer(cn))
}

// ── Waveshaper ─────────────────────────────────────────────────────────────

type cWaveshaper struct {
	w C.ifx_waveshaper_t
}

func newWaveshaper(_ int, params map[string]float64) *cWaveshaper {
	w := &cWaveshaper{}
	C.ifx_waveshaper_init(&w.w,
		C.float(params["curve"]), C.float(params["drive"]), C.float(params["mix"]))
	return w
}

func (w *cWaveshaper) ProcessSample(x float64) float64 {
	in := [1]C.float{C.float(x)}
	out := [1]C.float{}
	C.ifx_waveshaper_process(&w.w, &in[0], &out[0], 1)
	return float64(out[0])
}

func (w *cWaveshaper) ProcessBlockBuf(in []float32, out []float32, samples int) {
	if samples <= 0 {
		return
	}
	C.ifx_waveshaper_process(&w.w,
		(*C.float)(unsafe.Pointer(&in[0])),
		(*C.float)(unsafe.Pointer(&out[0])),
		C.int(samples))
}

func (w *cWaveshaper) Reset() { C.ifx_waveshaper_reset(&w.w) }
func (w *cWaveshaper) SetParam(name string, v float64) {
	cn := C.CString(name)
	C.ifx_waveshaper_set_param(&w.w, cn, C.float(v))
	C.free(unsafe.Pointer(cn))
}

// ── Ring Mod ───────────────────────────────────────────────────────────────

type cRingMod struct {
	r C.ifx_ringmod_t
}

func newRingMod(sr int, params map[string]float64) *cRingMod {
	r := &cRingMod{}
	C.ifx_ringmod_init(&r.r, C.int(sr),
		C.float(params["frequency"]), C.float(params["shape"]), C.float(params["mix"]))
	return r
}

func (r *cRingMod) ProcessSample(x float64) float64 {
	in := [1]C.float{C.float(x)}
	out := [1]C.float{}
	C.ifx_ringmod_process(&r.r, &in[0], &out[0], 1)
	return float64(out[0])
}

func (r *cRingMod) ProcessBlockBuf(in []float32, out []float32, samples int) {
	if samples <= 0 {
		return
	}
	C.ifx_ringmod_process(&r.r,
		(*C.float)(unsafe.Pointer(&in[0])),
		(*C.float)(unsafe.Pointer(&out[0])),
		C.int(samples))
}

func (r *cRingMod) Reset() { C.ifx_ringmod_reset(&r.r) }
func (r *cRingMod) SetParam(name string, v float64) {
	cn := C.CString(name)
	C.ifx_ringmod_set_param(&r.r, cn, C.float(v))
	C.free(unsafe.Pointer(cn))
}

// ── Tremolo ────────────────────────────────────────────────────────────────

type cTremolo struct {
	t C.ifx_tremolo_t
}

func newTremolo(sr int, params map[string]float64) *cTremolo {
	t := &cTremolo{}
	C.ifx_tremolo_init(&t.t, C.int(sr),
		C.float(params["rate"]), C.float(params["depth"]),
		C.float(params["shape"]), C.float(params["mix"]))
	return t
}

func (t *cTremolo) ProcessSample(x float64) float64 {
	in := [1]C.float{C.float(x)}
	out := [1]C.float{}
	C.ifx_tremolo_process(&t.t, &in[0], &out[0], 1)
	return float64(out[0])
}

func (t *cTremolo) ProcessBlockBuf(in []float32, out []float32, samples int) {
	if samples <= 0 {
		return
	}
	C.ifx_tremolo_process(&t.t,
		(*C.float)(unsafe.Pointer(&in[0])),
		(*C.float)(unsafe.Pointer(&out[0])),
		C.int(samples))
}

func (t *cTremolo) Reset() { C.ifx_tremolo_reset(&t.t) }
func (t *cTremolo) SetParam(name string, v float64) {
	cn := C.CString(name)
	C.ifx_tremolo_set_param(&t.t, cn, C.float(v))
	C.free(unsafe.Pointer(cn))
}

// ── Gate ───────────────────────────────────────────────────────────────────

type cGate struct {
	g C.ifx_gate_t
}

func newGate(sr int, params map[string]float64) *cGate {
	g := &cGate{}
	C.ifx_gate_init(&g.g, C.int(sr),
		C.float(params["threshold"]), C.float(params["attack"]),
		C.float(params["release"]), C.float(params["range"]))
	return g
}

func (g *cGate) ProcessSample(x float64) float64 {
	in := [1]C.float{C.float(x)}
	out := [1]C.float{}
	C.ifx_gate_process(&g.g, &in[0], &out[0], 1)
	return float64(out[0])
}

func (g *cGate) ProcessBlockBuf(in []float32, out []float32, samples int) {
	if samples <= 0 {
		return
	}
	C.ifx_gate_process(&g.g,
		(*C.float)(unsafe.Pointer(&in[0])),
		(*C.float)(unsafe.Pointer(&out[0])),
		C.int(samples))
}

func (g *cGate) Reset() { C.ifx_gate_reset(&g.g) }
func (g *cGate) SetParam(name string, v float64) {
	cn := C.CString(name)
	C.ifx_gate_set_param(&g.g, cn, C.float(v))
	C.free(unsafe.Pointer(cn))
}

// ── Limiter ────────────────────────────────────────────────────────────────

type cLimiter struct {
	l C.ifx_limiter_t
}

func newLimiter(sr int, params map[string]float64) *cLimiter {
	l := &cLimiter{}
	C.ifx_limiter_init(&l.l, C.int(sr),
		C.float(params["threshold"]), C.float(params["release"]),
		C.float(params["ceiling"]))
	return l
}

func (l *cLimiter) ProcessSample(x float64) float64 {
	in := [1]C.float{C.float(x)}
	out := [1]C.float{}
	C.ifx_limiter_process(&l.l, &in[0], &out[0], 1)
	return float64(out[0])
}

func (l *cLimiter) ProcessBlockBuf(in []float32, out []float32, samples int) {
	if samples <= 0 {
		return
	}
	C.ifx_limiter_process(&l.l,
		(*C.float)(unsafe.Pointer(&in[0])),
		(*C.float)(unsafe.Pointer(&out[0])),
		C.int(samples))
}

func (l *cLimiter) Reset() { C.ifx_limiter_reset(&l.l) }
func (l *cLimiter) SetParam(name string, v float64) {
	cn := C.CString(name)
	C.ifx_limiter_set_param(&l.l, cn, C.float(v))
	C.free(unsafe.Pointer(cn))
}

// ── Flanger ────────────────────────────────────────────────────────────────

type cFlanger struct {
	f      C.ifx_flanger_t
	bufPtr unsafe.Pointer
}

func newFlanger(sr int, params map[string]float64) *cFlanger {
	f := &cFlanger{}
	srF := float64(sr)
	if srF <= 0 {
		srF = 44100
	}
	// Max depth 10ms + margin
	bufLen := int(0.02*srF) + 64
	f.bufPtr = C.malloc(C.size_t(bufLen) * C.size_t(unsafe.Sizeof(C.float(0))))
	C.ifx_flanger_init(&f.f, C.int(sr), (*C.float)(f.bufPtr), C.int(bufLen),
		C.float(params["rate"]), C.float(params["depth"]),
		C.float(params["feedback"]), C.float(params["mix"]))
	return f
}

func (f *cFlanger) ProcessSample(x float64) float64 {
	in := [1]C.float{C.float(x)}
	out := [1]C.float{}
	C.ifx_flanger_process(&f.f, &in[0], &out[0], 1)
	return float64(out[0])
}

func (f *cFlanger) ProcessBlockBuf(in []float32, out []float32, samples int) {
	if samples <= 0 {
		return
	}
	C.ifx_flanger_process(&f.f,
		(*C.float)(unsafe.Pointer(&in[0])),
		(*C.float)(unsafe.Pointer(&out[0])),
		C.int(samples))
}

func (f *cFlanger) Reset() { C.ifx_flanger_reset(&f.f) }
func (f *cFlanger) SetParam(name string, v float64) {
	cn := C.CString(name)
	C.ifx_flanger_set_param(&f.f, cn, C.float(v))
	C.free(unsafe.Pointer(cn))
}

// ── Phaser ─────────────────────────────────────────────────────────────────

type cPhaser struct {
	p C.ifx_phaser_t
}

func newPhaser(sr int, params map[string]float64) *cPhaser {
	p := &cPhaser{}
	C.ifx_phaser_init(&p.p, C.int(sr),
		C.float(params["stages"]), C.float(params["rate"]),
		C.float(params["depth"]), C.float(params["feedback"]),
		C.float(params["mix"]))
	return p
}

func (p *cPhaser) ProcessSample(x float64) float64 {
	in := [1]C.float{C.float(x)}
	out := [1]C.float{}
	C.ifx_phaser_process(&p.p, &in[0], &out[0], 1)
	return float64(out[0])
}

func (p *cPhaser) ProcessBlockBuf(in []float32, out []float32, samples int) {
	if samples <= 0 {
		return
	}
	C.ifx_phaser_process(&p.p,
		(*C.float)(unsafe.Pointer(&in[0])),
		(*C.float)(unsafe.Pointer(&out[0])),
		C.int(samples))
}

func (p *cPhaser) Reset() { C.ifx_phaser_reset(&p.p) }
func (p *cPhaser) SetParam(name string, v float64) {
	cn := C.CString(name)
	C.ifx_phaser_set_param(&p.p, cn, C.float(v))
	C.free(unsafe.Pointer(cn))
}

// ── Auto-Wah ──────────────────────────────────────────────────────────────

type cAutoWah struct {
	a C.ifx_autowah_t
}

func newAutoWah(sr int, params map[string]float64) *cAutoWah {
	a := &cAutoWah{}
	C.ifx_autowah_init(&a.a, C.int(sr),
		C.float(params["sensitivity"]), C.float(params["rate"]),
		C.float(params["depth"]), C.float(params["mix"]))
	return a
}

func (a *cAutoWah) ProcessSample(x float64) float64 {
	in := [1]C.float{C.float(x)}
	out := [1]C.float{}
	C.ifx_autowah_process(&a.a, &in[0], &out[0], 1)
	return float64(out[0])
}

func (a *cAutoWah) ProcessBlockBuf(in []float32, out []float32, samples int) {
	if samples <= 0 {
		return
	}
	C.ifx_autowah_process(&a.a,
		(*C.float)(unsafe.Pointer(&in[0])),
		(*C.float)(unsafe.Pointer(&out[0])),
		C.int(samples))
}

func (a *cAutoWah) Reset() { C.ifx_autowah_reset(&a.a) }
func (a *cAutoWah) SetParam(name string, v float64) {
	cn := C.CString(name)
	C.ifx_autowah_set_param(&a.a, cn, C.float(v))
	C.free(unsafe.Pointer(cn))
}

// ── Compressor ─────────────────────────────────────────────────────────────

type cCompressorFX struct {
	c C.ifx_compressor_t
}

func newCompressorFX(sr int, params map[string]float64) *cCompressorFX {
	c := &cCompressorFX{}
	C.ifx_compressor_init(&c.c, C.int(sr),
		C.float(params["threshold"]), C.float(params["ratio"]),
		C.float(params["attack"]), C.float(params["release"]),
		C.float(params["makeup"]), C.float(params["mix"]))
	return c
}

func (c *cCompressorFX) ProcessSample(x float64) float64 {
	in := [1]C.float{C.float(x)}
	out := [1]C.float{}
	C.ifx_compressor_process(&c.c, &in[0], &out[0], 1)
	return float64(out[0])
}

func (c *cCompressorFX) ProcessBlockBuf(in []float32, out []float32, samples int) {
	if samples <= 0 {
		return
	}
	C.ifx_compressor_process(&c.c,
		(*C.float)(unsafe.Pointer(&in[0])),
		(*C.float)(unsafe.Pointer(&out[0])),
		C.int(samples))
}

func (c *cCompressorFX) Reset() { C.ifx_compressor_reset(&c.c) }
func (c *cCompressorFX) SetParam(name string, v float64) {
	cn := C.CString(name)
	C.ifx_compressor_set_param(&c.c, cn, C.float(v))
	C.free(unsafe.Pointer(cn))
}

// ── Transient Shaper ──────────────────────────────────────────────────────

type cTransient struct {
	t C.ifx_transient_t
}

func newTransient(sr int, params map[string]float64) *cTransient {
	t := &cTransient{}
	C.ifx_transient_init(&t.t, C.int(sr),
		C.float(params["attack"]), C.float(params["sustain"]),
		C.float(params["speed"]))
	return t
}

func (t *cTransient) ProcessSample(x float64) float64 {
	in := [1]C.float{C.float(x)}
	out := [1]C.float{}
	C.ifx_transient_process(&t.t, &in[0], &out[0], 1)
	return float64(out[0])
}

func (t *cTransient) ProcessBlockBuf(in []float32, out []float32, samples int) {
	if samples <= 0 {
		return
	}
	C.ifx_transient_process(&t.t,
		(*C.float)(unsafe.Pointer(&in[0])),
		(*C.float)(unsafe.Pointer(&out[0])),
		C.int(samples))
}

func (t *cTransient) Reset() { C.ifx_transient_reset(&t.t) }
func (t *cTransient) SetParam(name string, v float64) {
	cn := C.CString(name)
	C.ifx_transient_set_param(&t.t, cn, C.float(v))
	C.free(unsafe.Pointer(cn))
}

// ── Tape Saturation ───────────────────────────────────────────────────────

type cTape struct {
	t      C.ifx_tape_t
	bufPtr unsafe.Pointer
}

func newTape(sr int, params map[string]float64) *cTape {
	t := &cTape{}
	srF := float64(sr)
	if srF <= 0 {
		srF = 44100
	}
	// Max wow+flutter ~3ms, buffer needs ~6ms + margin
	bufLen := int(0.01*srF) + 64
	t.bufPtr = C.malloc(C.size_t(bufLen) * C.size_t(unsafe.Sizeof(C.float(0))))
	C.ifx_tape_init(&t.t, C.int(sr), (*C.float)(t.bufPtr), C.int(bufLen),
		C.float(params["drive"]), C.float(params["warmth"]),
		C.float(params["wow"]), C.float(params["flutter"]),
		C.float(params["mix"]))
	return t
}

func (t *cTape) ProcessSample(x float64) float64 {
	in := [1]C.float{C.float(x)}
	out := [1]C.float{}
	C.ifx_tape_process(&t.t, &in[0], &out[0], 1)
	return float64(out[0])
}

func (t *cTape) ProcessBlockBuf(in []float32, out []float32, samples int) {
	if samples <= 0 {
		return
	}
	C.ifx_tape_process(&t.t,
		(*C.float)(unsafe.Pointer(&in[0])),
		(*C.float)(unsafe.Pointer(&out[0])),
		C.int(samples))
}

func (t *cTape) Reset() { C.ifx_tape_reset(&t.t) }
func (t *cTape) SetParam(name string, v float64) {
	cn := C.CString(name)
	C.ifx_tape_set_param(&t.t, cn, C.float(v))
	C.free(unsafe.Pointer(cn))
}

// ── Pitch Shifter ─────────────────────────────────────────────────────────

type cPitchShift struct {
	p      C.ifx_pitchshift_t
	bufPtr unsafe.Pointer
}

func newPitchShift(sr int, params map[string]float64) *cPitchShift {
	p := &cPitchShift{}
	srF := float64(sr)
	if srF <= 0 {
		srF = 44100
	}
	// Window max 100ms, buffer 120ms + margin
	bufLen := int(0.15*srF) + 64
	p.bufPtr = C.malloc(C.size_t(bufLen) * C.size_t(unsafe.Sizeof(C.float(0))))
	C.ifx_pitchshift_init(&p.p, C.int(sr), (*C.float)(p.bufPtr), C.int(bufLen),
		C.float(params["pitch"]), C.float(params["mix"]),
		C.float(params["window"]))
	return p
}

func (p *cPitchShift) ProcessSample(x float64) float64 {
	in := [1]C.float{C.float(x)}
	out := [1]C.float{}
	C.ifx_pitchshift_process(&p.p, &in[0], &out[0], 1)
	return float64(out[0])
}

func (p *cPitchShift) ProcessBlockBuf(in []float32, out []float32, samples int) {
	if samples <= 0 {
		return
	}
	C.ifx_pitchshift_process(&p.p,
		(*C.float)(unsafe.Pointer(&in[0])),
		(*C.float)(unsafe.Pointer(&out[0])),
		C.int(samples))
}

func (p *cPitchShift) Reset() { C.ifx_pitchshift_reset(&p.p) }
func (p *cPitchShift) SetParam(name string, v float64) {
	cn := C.CString(name)
	C.ifx_pitchshift_set_param(&p.p, cn, C.float(v))
	C.free(unsafe.Pointer(cn))
}
