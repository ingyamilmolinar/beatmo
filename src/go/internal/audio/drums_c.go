//go:build !test && !js

package audio

/*
#cgo CFLAGS: -I${SRCDIR}/../../../c
#cgo LDFLAGS: -L${SRCDIR}/../../../../build -ldrums -lm
#include "drums.h"
#include <stdlib.h>
*/
import "C"
import (
	"errors"
	"fmt"
	"unsafe"
)

func renderSnare(buf []float32, sampleRate, samples int) {
	if samples > len(buf) {
		panic("renderSnare: samples exceeds buffer length")
	}
	if len(buf) == 0 || samples == 0 {
		return
	}
	C.render_snare((*C.float)(unsafe.Pointer(&buf[0])), C.int(sampleRate), C.int(samples))
}

func renderKick(buf []float32, sampleRate, samples int) {
	if samples > len(buf) {
		panic("renderKick: samples exceeds buffer length")
	}
	if len(buf) == 0 || samples == 0 {
		return
	}
	C.render_kick((*C.float)(unsafe.Pointer(&buf[0])), C.int(sampleRate), C.int(samples))
}

func renderHiHat(buf []float32, sampleRate, samples int) {
	if samples > len(buf) {
		panic("renderHiHat: samples exceeds buffer length")
	}
	if len(buf) == 0 || samples == 0 {
		return
	}
	C.render_hihat((*C.float)(unsafe.Pointer(&buf[0])), C.int(sampleRate), C.int(samples))
}

func renderTom(buf []float32, sampleRate, samples int) {
	if samples > len(buf) {
		panic("renderTom: samples exceeds buffer length")
	}
	if len(buf) == 0 || samples == 0 {
		return
	}
	C.render_tom((*C.float)(unsafe.Pointer(&buf[0])), C.int(sampleRate), C.int(samples))
}

func renderClap(buf []float32, sampleRate, samples int) {
	if samples > len(buf) {
		panic("renderClap: samples exceeds buffer length")
	}
	if len(buf) == 0 || samples == 0 {
		return
	}
	C.render_clap((*C.float)(unsafe.Pointer(&buf[0])), C.int(sampleRate), C.int(samples))
}

func loadWav(path string) ([]float32, int, error) {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	var ptr *C.float
	var sr C.int
	frames := C.load_wav(cpath, &ptr, &sr)
	if frames < 0 {
		msg := C.GoString(C.result_description(C.int(frames)))
		return nil, 0, fmt.Errorf("load_wav: %s", msg)
	}
	if frames == 0 {
		return nil, 0, errors.New("load_wav: no frames")
	}
	tmp := unsafe.Slice((*float32)(unsafe.Pointer(ptr)), int(frames))
	buf := make([]float32, int(frames))
	copy(buf, tmp)
	C.free(unsafe.Pointer(ptr))
	return buf, int(sr), nil
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
