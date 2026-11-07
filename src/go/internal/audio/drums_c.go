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

func loadAudio(path string) ([]float32, int, error) {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	var ptr *C.float
	var sr C.int
	// Rate 0 decodes at the file's OWN rate: conversion is pcmAtEngineRate, the same Go the browser runs on the same PCM,
	// never two different resamplers.
	frames := C.load_audio(cpath, 0, &ptr, &sr) //nolint:gocritic // dupSubExpr false positive in CGo
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

// Bulk conversion, no per-sample call overhead.
func (v *cVoice) SampleBlock(dst []float64) (int, bool) {
	remaining := len(v.buf) - v.i
	if remaining <= 0 {
		return 0, true
	}
	n := len(dst)
	if n > remaining {
		n = remaining
	}
	for j := 0; j < n; j++ {
		dst[j] = float64(v.buf[v.i+j])
	}
	v.i += n
	return n, v.i >= len(v.buf)
}
