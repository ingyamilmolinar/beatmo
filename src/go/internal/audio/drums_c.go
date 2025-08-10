//go:build !test && !js

package audio

/*
#cgo CFLAGS: -I${SRCDIR}/../../../c -DMA_NO_DEVICE_IO -DMA_NO_THREADING
#cgo LDFLAGS: -lm
#include "../../../c/miniaudio.c"
#include "../../../c/drums.c"
*/
import "C"
import "unsafe"

func renderSnare(buf []float32, sampleRate, samples int) {
	C.render_snare((*C.float)(unsafe.Pointer(&buf[0])), C.int(sampleRate), C.int(samples))
}

func renderKick(buf []float32, sampleRate, samples int) {
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
