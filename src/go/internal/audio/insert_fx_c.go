//go:build !test && !js

package audio

/*
#cgo CFLAGS: -I${SRCDIR}/../../../c
#cgo LDFLAGS: -L${SRCDIR}/../../../../build -ldrums -lm
#include "fx_host.h"
#include <stdlib.h>
*/
import "C"

import (
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// The same fx_host the browser's beatmo-dsp worklet drives, with the same complete params: one computation on both
// platforms.
type cEffect struct {
	t EffectType
	h unsafe.Pointer // nil when the C engine refused the effect: passes through
}

var cEffectParamNames = struct {
	once  sync.Once
	names map[EffectType][]string
}{}

// The order the C init reads them lives only in C (ifx_param_names).
func effectParamNames(t EffectType) []string {
	cEffectParamNames.once.Do(func() {
		m := make(map[EffectType][]string, len(EffectTypes))
		for _, et := range EffectTypes {
			ct := C.CString(string(et))
			s := C.GoString(C.ifx_param_names(ct))
			C.free(unsafe.Pointer(ct))
			if s != "" {
				m[et] = strings.Split(s, ",")
			}
		}
		cEffectParamNames.names = m
	})
	return cEffectParamNames.names[t]
}

func newCEffect(t EffectType, sr int, params map[string]float64) *cEffect {
	p := mergeDefaults(t, params)
	names := effectParamNames(t)
	vals := make([]C.float, len(names)+1) // +1: never a nil pointer for C
	for i, n := range names {
		vals[i] = C.float(p[n])
	}
	ct := C.CString(string(t))
	h := C.ifx_create(ct, C.int(sr), &vals[0], C.int(len(names)))
	C.free(unsafe.Pointer(ct))
	e := &cEffect{t: t, h: h}
	if h == nil {
		log.Warnf("[audio] insert effect %q could not be created at %d Hz: it passes audio through", t, sr)
		return e
	}
	runtime.SetFinalizer(e, func(e *cEffect) { C.ifx_destroy(e.h) })
	return e
}

func (e *cEffect) ProcessSample(x float64) float64 {
	if e.h == nil {
		return x
	}
	in := [1]C.float{C.float(x)}
	out := [1]C.float{}
	C.ifx_run(e.h, &in[0], &out[0], 1)
	return float64(out[0])
}

func (e *cEffect) ProcessBlockBuf(in []float32, out []float32, samples int) {
	if samples <= 0 {
		return
	}
	if e.h == nil {
		copy(out[:samples], in[:samples])
		return
	}
	C.ifx_run(e.h, (*C.float)(unsafe.Pointer(&in[0])), (*C.float)(unsafe.Pointer(&out[0])), C.int(samples))
}

func (e *cEffect) Reset() {
	if e.h != nil {
		C.ifx_clear(e.h)
	}
}

func (e *cEffect) SetParam(name string, v float64) {
	if e.h == nil {
		return
	}
	cn := C.CString(name)
	C.ifx_set(e.h, cn, C.float(v))
	C.free(unsafe.Pointer(cn))
}

// Every type is the generic C effect on desktop.
func newDistortion(sr int, p map[string]float64) *cEffect { return newCEffect(EffectDistortion, sr, p) }
func newDelay(sr int, p map[string]float64) *cEffect      { return newCEffect(EffectDelay, sr, p) }
func newReverb(sr int, p map[string]float64) *cEffect     { return newCEffect(EffectReverb, sr, p) }
func newChorus(sr int, p map[string]float64) *cEffect     { return newCEffect(EffectChorus, sr, p) }
func newBitcrusher(sr int, p map[string]float64) *cEffect { return newCEffect(EffectBitcrusher, sr, p) }
func newFilter(sr int, p map[string]float64) *cEffect     { return newCEffect(EffectFilter, sr, p) }
func newWaveshaper(sr int, p map[string]float64) *cEffect { return newCEffect(EffectWaveshaper, sr, p) }
func newRingMod(sr int, p map[string]float64) *cEffect    { return newCEffect(EffectRingMod, sr, p) }
func newTremolo(sr int, p map[string]float64) *cEffect    { return newCEffect(EffectTremolo, sr, p) }
func newGate(sr int, p map[string]float64) *cEffect       { return newCEffect(EffectGate, sr, p) }
func newLimiter(sr int, p map[string]float64) *cEffect    { return newCEffect(EffectLimiter, sr, p) }
func newFlanger(sr int, p map[string]float64) *cEffect    { return newCEffect(EffectFlanger, sr, p) }
func newPhaser(sr int, p map[string]float64) *cEffect     { return newCEffect(EffectPhaser, sr, p) }
func newAutoWah(sr int, p map[string]float64) *cEffect    { return newCEffect(EffectAutoWah, sr, p) }
func newCompressorFX(sr int, p map[string]float64) *cEffect {
	return newCEffect(EffectCompressor, sr, p)
}
func newTransient(sr int, p map[string]float64) *cEffect  { return newCEffect(EffectTransient, sr, p) }
func newTape(sr int, p map[string]float64) *cEffect       { return newCEffect(EffectTape, sr, p) }
func newPitchShift(sr int, p map[string]float64) *cEffect { return newCEffect(EffectPitchShift, sr, p) }
