//go:build !test && !js

package audio

/*
#cgo CFLAGS: -I${SRCDIR}/../../../c
#cgo LDFLAGS: -L${SRCDIR}/../../../../build -ldrums -lm
#include "effects.h"
*/
import "C"
import "unsafe"

// sendDelayProcessForTest pushes a block of samples through the global send
// delay so tests can ring the wet tail before/after a smooth param change.
// Exists only because Go forbids CGo in *_test.go files.
func sendDelayProcessForTest(buf []float32) {
	if len(buf) == 0 || sendFX == nil {
		return
	}
	C.delay_process(&sendFX.delay, (*C.float)(unsafe.Pointer(&buf[0])), C.int(len(buf)))
}

// sendReverbProcessForTest pushes a block through the global send reverb.
func sendReverbProcessForTest(in, out []float32) {
	if len(in) == 0 || len(out) == 0 || sendFX == nil {
		return
	}
	C.reverb_process(&sendFX.reverb, (*C.float)(unsafe.Pointer(&in[0])),
		(*C.float)(unsafe.Pointer(&out[0])), C.int(len(in)))
}
