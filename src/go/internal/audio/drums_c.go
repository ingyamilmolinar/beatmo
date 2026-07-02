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

// renderSnare deleted — the snare family migrated to the modular engine
// (Phase-5). The no-edit native fast path is renderSnareVoice
// (snare_modular_native.go), which renders the baked recipe-default
// ModularParams through render_modular_p.

// renderKick deleted — the base kick migrated to the modular engine (Phase-3).
// The no-edit native fast path is renderKickVoice (kick_modular_native.go),
// which renders the baked recipe-default ModularParams through render_modular_p.

// renderHiHat / renderOpenHiHat / renderCowbell / renderShaker / renderRide /
// renderCrash deleted — the cymbal family migrated to the modular engine
// (Phase-6). The no-edit native fast paths are renderHiHatVoice /
// renderOpenHiHatVoice / renderCowbellVoice / renderShakerVoice / renderRideVoice
// / renderCrashVoice (cymbal_modular_native.go), which render the baked
// recipe-default ModularParams through render_modular_p; the recipe/edit path
// renders through the modular binding (cymbalRecipeToModular).

// renderTom / renderTomHigh / renderTomLow deleted — the tom family migrated to
// the modular engine (Phase-4). The no-edit fast path is renderTomVoice /
// renderTomHighVoice / renderTomLowVoice (tom_modular_native.go); the recipe/edit
// path renders through the modular binding (tomRecipeToModular).

// renderClap deleted — the clap migrated to the modular engine (Phase-5). The
// no-edit native fast path is renderClapVoice (snare_modular_native.go).

// renderBassGuitar / renderSubBass deleted — the bass family migrated to the
// modular engine (Phase-2). The no-edit native fast path is renderSubBassVoice
// / renderSubBassVoice (bass_modular_native.go), which renders the baked
// recipe-default ModularParams through render_modular_p.

// renderSnareRimshot / renderSnareSidestick deleted — the snare family migrated
// to the modular engine (Phase-5). The no-edit native fast paths are
// renderSnareRimshotVoice / renderSnareSidestickVoice (snare_modular_native.go).

// renderKickDeep / renderKickPunchy / renderKickLofi / renderKickTight deleted —
// the kick family migrated to the modular engine (Phase-3). The no-edit native
// fast paths are renderKickDeepVoice / renderKickPunchyVoice /
// renderKickLofiVoice / renderKickTightVoice (kick_modular_native.go).

func loadAudio(path string) ([]float32, int, error) {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	var ptr *C.float
	var sr C.int
	frames := C.load_audio(cpath, C.int(sampleRate), &ptr, &sr) //nolint:gocritic // dupSubExpr false positive in CGo
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

// SampleBlock performs bulk float32→float64 conversion, avoiding per-sample
// function call overhead. Implements the BlockVoice interface.
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
