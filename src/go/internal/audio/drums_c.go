//go:build !test && !js

package audio

/*
#cgo CFLAGS: -I${SRCDIR}/../../../c
#cgo LDFLAGS: -L${SRCDIR}/../../../../build -ldrums -lm
#include "drums.h"
*/
import "C"
import "unsafe"

func renderSnare(buf []float32, sampleRate, samples int) {
    if samples > len(buf) {
        panic("renderSnare: samples exceeds buffer length")
    }
    C.render_snare((*C.float)(unsafe.Pointer(&buf[0])), C.int(sampleRate), C.int(samples))
}

func renderKick(buf []float32, sampleRate, samples int) {
    if samples > len(buf) {
        panic("renderKick: samples exceeds buffer length")
    }
    C.render_kick((*C.float)(unsafe.Pointer(&buf[0])), C.int(sampleRate), C.int(samples))
}

type cVoice struct {
    buf []float32
    i   int
}

func (v *cVoice) Sample() (float64, bool) {
    if v.i >= len(v.buf) {
        return 0, true
    }
    f := float64(v.buf[v.i])
    v.i++
    return f, false
}
