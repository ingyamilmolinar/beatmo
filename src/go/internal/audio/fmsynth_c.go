//go:build !test && !js

package audio

/*
#cgo CFLAGS: -I${SRCDIR}/../../../c
#cgo LDFLAGS: -L${SRCDIR}/../../../../build -ldrums -lm
#include "fmsynth.h"
*/
import "C"
import "unsafe"

func renderFMBass(buf []float32, sampleRate, samples int) {
	if samples > len(buf) {
		panic("renderFMBass: samples exceeds buffer length")
	}
	if len(buf) == 0 || samples == 0 {
		return
	}
	C.render_fm_bass((*C.float)(unsafe.Pointer(&buf[0])), C.int(sampleRate), C.int(samples))
}

func renderFMBell(buf []float32, sampleRate, samples int) {
	if samples > len(buf) {
		panic("renderFMBell: samples exceeds buffer length")
	}
	if len(buf) == 0 || samples == 0 {
		return
	}
	C.render_fm_bell((*C.float)(unsafe.Pointer(&buf[0])), C.int(sampleRate), C.int(samples))
}

func renderFMLead(buf []float32, sampleRate, samples int) {
	if samples > len(buf) {
		panic("renderFMLead: samples exceeds buffer length")
	}
	if len(buf) == 0 || samples == 0 {
		return
	}
	C.render_fm_lead((*C.float)(unsafe.Pointer(&buf[0])), C.int(sampleRate), C.int(samples))
}

func renderFMEPiano(buf []float32, sampleRate, samples int) {
	if samples > len(buf) {
		panic("renderFMEPiano: samples exceeds buffer length")
	}
	if len(buf) == 0 || samples == 0 {
		return
	}
	C.render_fm_epiano((*C.float)(unsafe.Pointer(&buf[0])), C.int(sampleRate), C.int(samples))
}

func renderFMPluck(buf []float32, sampleRate, samples int) {
	if samples > len(buf) {
		panic("renderFMPluck: samples exceeds buffer length")
	}
	if len(buf) == 0 || samples == 0 {
		return
	}
	C.render_fm_pluck((*C.float)(unsafe.Pointer(&buf[0])), C.int(sampleRate), C.int(samples))
}
