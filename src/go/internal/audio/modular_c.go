//go:build !test && !js

package audio

/*
#cgo CFLAGS: -I${SRCDIR}/../../../c
#cgo LDFLAGS: -L${SRCDIR}/../../../../build -ldrums -lm
#include "modular.h"
#include "noise.h"
void oracle_ma_noise_white_fill(float *out, int n, int seed);
*/
import "C"
import (
	"fmt"
	"unsafe"
)

// Element i is ModularParamSchema()[i]; the struct is a dense array of doubles (static-asserted), handed to C as-is.
type modularBlock []float64

var modularSchemaIndex = ModularParamSchemaIndexMap()

func zeroModularBlock() modularBlock { return make(modularBlock, len(modularParamSchema)) }

// A block is a slice: plain assignment aliases it.
func (b modularBlock) clone() modularBlock { return append(modularBlock(nil), b...) }

func (b modularBlock) set(name string, v float64) {
	i, ok := modularSchemaIndex[name]
	if !ok {
		panic("modularBlock.set: unknown param " + name)
	}
	b[i] = v
}

func (b modularBlock) get(name string) float64 {
	i, ok := modularSchemaIndex[name]
	if !ok {
		panic("modularBlock.get: unknown param " + name)
	}
	return b[i]
}

func compiledModularParamCount() int { return int(C.modular_param_count()) }

// A libdrums.a compiled against an older header reads later fields at shifted offsets, wrong voices with no error, so
// it is refused (`make build/libdrums.a` rebuilds it).
func init() {
	if n := compiledModularParamCount(); n != len(modularParamSchema) {
		panic(fmt.Sprintf("audio: the linked C engine has %d params but the schema has %d — rebuild build/libdrums.a", n, len(modularParamSchema)))
	}
}

// renderModularBlock renders the modular voice into buf from a param block.
func renderModularBlock(buf []float32, sampleRate, samples int, b modularBlock) {
	if samples > len(buf) || len(buf) == 0 || samples == 0 || len(b) != len(modularParamSchema) {
		return
	}
	if renderModularRoundParamsToFloat32 {
		b = roundModularParamsToFloat32(b)
	}
	C.render_modular_p((*C.float)(unsafe.Pointer(&buf[0])), C.int(sampleRate), C.int(samples),
		(*C.modular_params)(unsafe.Pointer(&b[0])))
}

// The test seam that measures the post ops against a known signal.
func applyModularFinish(buf []float32, sampleRate int, b modularBlock) {
	if len(buf) == 0 || len(b) != len(modularParamSchema) {
		return
	}
	C.modular_finish_apply((*C.float)(unsafe.Pointer(&buf[0])), C.int(sampleRate), C.int(len(buf)),
		(*C.modular_params)(unsafe.Pointer(&b[0])))
}

// Test oracle only: production uses noiseMaWhiteFill.
func oracleMaNoiseWhiteFill(out []float32, seed int) {
	if len(out) == 0 {
		return
	}
	C.oracle_ma_noise_white_fill((*C.float)(unsafe.Pointer(&out[0])), C.int(len(out)), C.int(seed))
}

// noiseMaWhiteFill fills out with the transplanted deterministic stream.
func noiseMaWhiteFill(out []float32, seed int) {
	if len(out) == 0 {
		return
	}
	C.noise_ma_white_fill((*C.float)(unsafe.Pointer(&out[0])), C.int(len(out)), C.int(seed))
}
