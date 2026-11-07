//go:build !test && !js

package audio

/*
#cgo CFLAGS: -I${SRCDIR}/../../../c
#cgo LDFLAGS: -L${SRCDIR}/../../../../build -ldrums -lm
#include "mix_dsp.h"
*/
import "C"

import (
	"runtime"
	"sync"
	"unsafe"
)

// The mutex orders a UI-thread retune against the audio thread's block.
type cEQ struct {
	mu sync.Mutex
	h  unsafe.Pointer
}

func newCEQ(prog []float64) *cEQ {
	e := &cEQ{}
	if len(prog) > 0 {
		e.h = C.eq_create((*C.double)(unsafe.Pointer(&prog[0])), C.int(len(prog)))
	}
	if e.h != nil {
		runtime.SetFinalizer(e, func(e *cEQ) { C.eq_destroy(e.h) })
	}
	return e
}

// update swaps in prog's values keeping filter state when its shape matches.
func (e *cEQ) update(prog []float64) bool {
	if e.h == nil || len(prog) == 0 {
		return false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return C.eq_update(e.h, (*C.double)(unsafe.Pointer(&prog[0])), C.int(len(prog))) == 1
}

func (e *cEQ) ProcessSample(x float64) float64 {
	if e.h == nil {
		return x
	}
	in := [1]C.float{C.float(x)}
	out := [1]C.float{}
	e.mu.Lock()
	C.eq_run(e.h, &in[0], &out[0], 1)
	e.mu.Unlock()
	return float64(out[0])
}

func (e *cEQ) ProcessBlockBuf(in, out []float32, samples int) {
	if samples <= 0 {
		return
	}
	if e.h == nil {
		copy(out[:samples], in[:samples])
		return
	}
	e.mu.Lock()
	C.eq_run(e.h, (*C.float)(unsafe.Pointer(&in[0])), (*C.float)(unsafe.Pointer(&out[0])), C.int(samples))
	e.mu.Unlock()
}

// Reset clears every filter's delay line. See audio.ResetDSPState.
func (e *cEQ) Reset() {
	if e.h == nil {
		return
	}
	e.mu.Lock()
	C.eq_clear(e.h)
	e.mu.Unlock()
}

// The browser runs the same C.
func softClipBlock(buf []float64) {
	if len(buf) == 0 {
		return
	}
	p := (*C.double)(unsafe.Pointer(&buf[0]))
	C.soft_clip_run_d(p, p, C.int(len(buf)), C.double(SoftClipThreshold))
}
