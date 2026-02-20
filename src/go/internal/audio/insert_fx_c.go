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
