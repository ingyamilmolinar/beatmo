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

func renderOpenHiHat(buf []float32, sampleRate, samples int) {
	if samples > len(buf) {
		panic("renderOpenHiHat: samples exceeds buffer length")
	}
	if len(buf) == 0 || samples == 0 {
		return
	}
	C.render_open_hihat((*C.float)(unsafe.Pointer(&buf[0])), C.int(sampleRate), C.int(samples))
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

func renderTomHigh(buf []float32, sampleRate, samples int) {
	if samples > len(buf) {
		panic("renderTomHigh: samples exceeds buffer length")
	}
	if len(buf) == 0 || samples == 0 {
		return
	}
	C.render_tom_high((*C.float)(unsafe.Pointer(&buf[0])), C.int(sampleRate), C.int(samples))
}

func renderTomLow(buf []float32, sampleRate, samples int) {
	if samples > len(buf) {
		panic("renderTomLow: samples exceeds buffer length")
	}
	if len(buf) == 0 || samples == 0 {
		return
	}
	C.render_tom_low((*C.float)(unsafe.Pointer(&buf[0])), C.int(sampleRate), C.int(samples))
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

func renderCowbell(buf []float32, sampleRate, samples int) {
	if samples > len(buf) {
		panic("renderCowbell: samples exceeds buffer length")
	}
	if len(buf) == 0 || samples == 0 {
		return
	}
	C.render_cowbell((*C.float)(unsafe.Pointer(&buf[0])), C.int(sampleRate), C.int(samples))
}

func renderBassGuitar(buf []float32, sampleRate, samples int) {
	if samples > len(buf) {
		panic("renderBassGuitar: samples exceeds buffer length")
	}
	if len(buf) == 0 || samples == 0 {
		return
	}
	C.render_bass_guitar((*C.float)(unsafe.Pointer(&buf[0])), C.int(sampleRate), C.int(samples))
}

func renderSubBass(buf []float32, sampleRate, samples int) {
	if samples > len(buf) {
		panic("renderSubBass: samples exceeds buffer length")
	}
	if len(buf) == 0 || samples == 0 {
		return
	}
	C.render_sub_bass((*C.float)(unsafe.Pointer(&buf[0])), C.int(sampleRate), C.int(samples))
}

func loadAudio(path string) ([]float32, int, error) {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	var ptr *C.float
	var sr C.int
	frames := C.load_audio(cpath, &ptr, &sr)
	if frames < 0 {
		msg := C.GoString(C.result_description(C.int(frames)))
		return nil, 0, fmt.Errorf("load_audio: %s", msg)
	}
	if frames == 0 {
		return nil, 0, errors.New("load_audio: no frames")
	}
	if ptr == nil {
		return nil, 0, errors.New("load_audio: C returned nil pointer")
	}
	tmp := unsafe.Slice((*float32)(unsafe.Pointer(ptr)), int(frames))
	buf := make([]float32, int(frames))
	copy(buf, tmp)
	C.free(unsafe.Pointer(ptr))
	return buf, int(sr), nil
}

// Backward-compatible alias for callers expecting a WAV-specific name.
func loadWav(path string) ([]float32, int, error) {
	return loadAudio(path)
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
