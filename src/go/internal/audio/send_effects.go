//go:build !test && !js

package audio

/*
#cgo CFLAGS: -I${SRCDIR}/../../../c
#cgo LDFLAGS: -L${SRCDIR}/../../../../build -ldrums -lm
#include "effects.h"
#include <stdlib.h>
*/
import "C"
import (
	"math"
	"sync"
	"sync/atomic"
	"unsafe"
)

// sendEffects manages the delay and reverb send buses.
// Each instrument channel has send levels (0-1) that determine how much
// signal is tapped and routed to each effect.
type sendEffects struct {
	mu sync.Mutex

	// C effect structs.
	delay  C.delay_t
	reverb C.reverb_t

	// Owned C buffers (freed on cleanup).
	delayBuf        unsafe.Pointer
	delayCapSamples int // capacity of delayBuf in floats; max supported delay time.
	reverbBuf       unsafe.Pointer

	// Go-side processing buffers.
	delaySendBuf  []float64 // accumulated send to delay
	reverbSendBuf []float64 // accumulated send to reverb
	delayOutBuf   []float32 // C delay output (float32)
	reverbInBuf   []float32 // C reverb input (float32)
	reverbOutBuf  []float32 // C reverb output (float32)

	sr          int
	initialized bool

	// Stored configuration for export/query.
	delayTimeMs    float64
	delayFeedback  float64
	delayDampingHz float64
	reverbRoom     float64
	reverbDamping  float64
	reverbWet      float64
}

// Per-instrument send levels stored atomically for lock-free reads.
type sendLevels struct {
	delay  uint64 // atomic float64 bits
	reverb uint64 // atomic float64 bits
}

var (
	sendFX       *sendEffects
	sendLevelMu  sync.RWMutex
	sendLevelMap = make(map[string]*sendLevels)
)

func getSendLevels(id string) *sendLevels {
	sendLevelMu.RLock()
	sl := sendLevelMap[id]
	sendLevelMu.RUnlock()
	if sl != nil {
		return sl
	}
	sendLevelMu.Lock()
	sl = sendLevelMap[id]
	if sl == nil {
		sl = &sendLevels{}
		sendLevelMap[id] = sl
	}
	sendLevelMu.Unlock()
	return sl
}

// SetDelaySend sets the delay send amount (0-1) for an instrument channel.
func SetDelaySend(id string, amount float64) {
	if amount < 0 {
		amount = 0
	}
	if amount > 1 {
		amount = 1
	}
	sl := getSendLevels(id)
	atomic.StoreUint64(&sl.delay, math.Float64bits(amount))
}

// DelaySend returns the delay send amount for an instrument.
func DelaySend(id string) float64 {
	sl := getSendLevels(id)
	return math.Float64frombits(atomic.LoadUint64(&sl.delay))
}

// SetReverbSend sets the reverb send amount (0-1) for an instrument channel.
func SetReverbSend(id string, amount float64) {
	if amount < 0 {
		amount = 0
	}
	if amount > 1 {
		amount = 1
	}
	sl := getSendLevels(id)
	atomic.StoreUint64(&sl.reverb, math.Float64bits(amount))
}

// ReverbSend returns the reverb send amount for an instrument.
func ReverbSend(id string) float64 {
	sl := getSendLevels(id)
	return math.Float64frombits(atomic.LoadUint64(&sl.reverb))
}

// maxSendDelayMs caps the delay-buffer allocation. The send delay UI clamps
// to this; ConfigureSendDelay will silently clamp larger requests. Living
// with a fixed capacity means delay_set_time_smooth never has to reallocate
// the C buffer, so time changes are click-free (Phase 7).
const maxSendDelayMs = 2000

// initSendEffects creates and initializes the global send effects.
func initSendEffects(sr int) {
	fx := &sendEffects{sr: sr}

	// Delay: allocate at the *maximum* supported time so ConfigureSendDelay
	// only ever updates the read offset, never reallocates the buffer. The
	// initial active length is 300ms (matches prior default behavior).
	maxDelaySamples := sr * maxSendDelayMs / 1000
	if maxDelaySamples < 1 {
		maxDelaySamples = 1
	}
	initialDelaySamples := sr * 300 / 1000
	if initialDelaySamples < 1 {
		initialDelaySamples = 1
	}
	fx.delayBuf = C.malloc(C.size_t(maxDelaySamples) * C.size_t(unsafe.Sizeof(C.float(0))))
	C.delay_init(&fx.delay, (*C.float)(fx.delayBuf), C.int(initialDelaySamples),
		C.float(0.3), C.float(3000), C.int(sr))
	fx.delayCapSamples = maxDelaySamples

	// Reverb: medium room, moderate damping.
	reverbSize := int(C.reverb_buffer_size(C.int(sr)))
	fx.reverbBuf = C.malloc(C.size_t(reverbSize) * C.size_t(unsafe.Sizeof(C.float(0))))
	C.reverb_init(&fx.reverb, (*C.float)(fx.reverbBuf), C.int(sr),
		C.float(0.7), C.float(0.4), C.float(0.3))

	// Allocate Go processing buffers.
	fx.delaySendBuf = make([]float64, blockSize)
	fx.reverbSendBuf = make([]float64, blockSize)
	fx.delayOutBuf = make([]float32, blockSize)
	fx.reverbInBuf = make([]float32, blockSize)
	fx.reverbOutBuf = make([]float32, blockSize)

	// Store default configuration for export/query.
	fx.delayTimeMs = 300
	fx.delayFeedback = 0.3
	fx.delayDampingHz = 3000
	fx.reverbRoom = 0.7
	fx.reverbDamping = 0.4
	fx.reverbWet = 0.3

	fx.initialized = true
	sendFX = fx
}

// processSlotSends is the slot-indexed variant of processSends, used by the
// mixer's hot path to avoid map lookups. slotIDs maps slot → instrument ID.
func (fx *sendEffects) processSlotSends(instBufs [][]float64, slotIDs []string, activeSlots []int, blockLen int, masterBuf []float64) {
	if !fx.initialized {
		return
	}

	// Zero send accumulation buffers.
	for i := 0; i < blockLen; i++ {
		fx.delaySendBuf[i] = 0
		fx.reverbSendBuf[i] = 0
	}

	// Accumulate send signals from active instruments.
	hasSend := false
	for _, slot := range activeSlots {
		id := slotIDs[slot]
		sl := getSendLevels(id)
		dlyAmt := math.Float64frombits(atomic.LoadUint64(&sl.delay))
		revAmt := math.Float64frombits(atomic.LoadUint64(&sl.reverb))
		if dlyAmt <= 0 && revAmt <= 0 {
			continue
		}
		hasSend = true
		buf := instBufs[slot]
		if dlyAmt > 0 {
			for j := 0; j < blockLen; j++ {
				fx.delaySendBuf[j] += buf[j] * dlyAmt
			}
		}
		if revAmt > 0 {
			for j := 0; j < blockLen; j++ {
				fx.reverbSendBuf[j] += buf[j] * revAmt
			}
		}
	}

	if !hasSend {
		return
	}

	// Process delay.
	for i := 0; i < blockLen; i++ {
		fx.delayOutBuf[i] = float32(fx.delaySendBuf[i])
	}
	C.delay_process(&fx.delay, (*C.float)(unsafe.Pointer(&fx.delayOutBuf[0])), C.int(blockLen))
	for i := 0; i < blockLen; i++ {
		masterBuf[i] += float64(fx.delayOutBuf[i])
	}

	// Process reverb.
	for i := 0; i < blockLen; i++ {
		fx.reverbInBuf[i] = float32(fx.reverbSendBuf[i])
	}
	C.reverb_process(&fx.reverb, (*C.float)(unsafe.Pointer(&fx.reverbInBuf[0])),
		(*C.float)(unsafe.Pointer(&fx.reverbOutBuf[0])), C.int(blockLen))
	for i := 0; i < blockLen; i++ {
		masterBuf[i] += float64(fx.reverbOutBuf[i])
	}
}

// processSendEffects runs in Phase 2.5: taps per-instrument buffers at send
// levels, processes through C delay/reverb, and mixes wet signal into masterBuf.
// Called from processBlock between Phase 2 and Phase 3.
func (fx *sendEffects) processSends(instBufs map[string][]float64, activeInsts []string, blockLen int, masterBuf []float64) {
	if !fx.initialized {
		return
	}

	// Zero send accumulation buffers.
	for i := 0; i < blockLen; i++ {
		fx.delaySendBuf[i] = 0
		fx.reverbSendBuf[i] = 0
	}

	// Accumulate send signals from active instruments.
	hasSend := false
	for _, id := range activeInsts {
		sl := getSendLevels(id)
		dlyAmt := math.Float64frombits(atomic.LoadUint64(&sl.delay))
		revAmt := math.Float64frombits(atomic.LoadUint64(&sl.reverb))
		if dlyAmt <= 0 && revAmt <= 0 {
			continue
		}
		hasSend = true
		buf := instBufs[id]
		if dlyAmt > 0 {
			for j := 0; j < blockLen; j++ {
				fx.delaySendBuf[j] += buf[j] * dlyAmt
			}
		}
		if revAmt > 0 {
			for j := 0; j < blockLen; j++ {
				fx.reverbSendBuf[j] += buf[j] * revAmt
			}
		}
	}

	if !hasSend {
		return
	}

	// Process delay.
	for i := 0; i < blockLen; i++ {
		fx.delayOutBuf[i] = float32(fx.delaySendBuf[i])
	}
	C.delay_process(&fx.delay, (*C.float)(unsafe.Pointer(&fx.delayOutBuf[0])), C.int(blockLen))
	for i := 0; i < blockLen; i++ {
		masterBuf[i] += float64(fx.delayOutBuf[i])
	}

	// Process reverb.
	for i := 0; i < blockLen; i++ {
		fx.reverbInBuf[i] = float32(fx.reverbSendBuf[i])
	}
	C.reverb_process(&fx.reverb, (*C.float)(unsafe.Pointer(&fx.reverbInBuf[0])),
		(*C.float)(unsafe.Pointer(&fx.reverbOutBuf[0])), C.int(blockLen))
	for i := 0; i < blockLen; i++ {
		masterBuf[i] += float64(fx.reverbOutBuf[i])
	}
}

// ConfigureSendDelay reconfigures the global send delay with new parameters.
// timeMs is the delay time in milliseconds (clamped to (0, maxSendDelayMs]
// because the C buffer is sized to maxSendDelayMs at init time and never
// reallocates — that's what makes the time update click-free). feedback
// is 0-1, dampingHz is the LP damping cutoff frequency. Safe to call at
// any time; acquires the send mutex. The smooth setters do not zero the
// delay buffer, so the existing wet tail keeps playing as the read offset
// shifts to the new time.
func ConfigureSendDelay(timeMs, feedback, dampingHz float64) {
	if sendFX == nil || !sendFX.initialized {
		return
	}
	sendFX.mu.Lock()
	defer sendFX.mu.Unlock()

	sr := sendFX.sr
	if timeMs > maxSendDelayMs {
		timeMs = maxSendDelayMs
	}
	delaySamples := int(float64(sr) * timeMs / 1000)
	if delaySamples < 1 {
		delaySamples = 1
	}
	if delaySamples > sendFX.delayCapSamples {
		delaySamples = sendFX.delayCapSamples
	}

	fb := feedback
	if fb < 0 {
		fb = 0
	} else if fb > 1 {
		fb = 1
	}

	C.delay_set_time_smooth(&sendFX.delay, C.int(delaySamples))
	C.delay_set_feedback_smooth(&sendFX.delay, C.float(fb))
	C.delay_set_damping_smooth(&sendFX.delay, C.float(dampingHz), C.int(sr))

	sendFX.delayTimeMs = timeMs
	sendFX.delayFeedback = fb
	sendFX.delayDampingHz = dampingHz
}

// ConfigureSendReverb reconfigures the global send reverb with new parameters.
// room is room size (0-1), damping is damping amount (0-1), wet is wet mix (0-1).
// Safe to call at any time; acquires the send mutex.
func ConfigureSendReverb(room, damping, wet float64) {
	if sendFX == nil || !sendFX.initialized {
		return
	}
	sendFX.mu.Lock()
	defer sendFX.mu.Unlock()

	sr := sendFX.sr
	r := clampSend(room, 0, 1)
	d := clampSend(damping, 0, 1)
	w := clampSend(wet, 0, 1)
	_ = sr // sr unused after switching to smooth setter; kept for symmetry with sendFX access

	// Smooth update: only mutates coefficients in place, the existing reverb
	// tail keeps playing instead of getting zeroed (which is what
	// reverb_init does and why the prior code clicked).
	C.reverb_set_params_smooth(&sendFX.reverb, C.float(r), C.float(d), C.float(w))

	sendFX.reverbRoom = r
	sendFX.reverbDamping = d
	sendFX.reverbWet = w
}

// SendDelayParams returns the current delay send configuration.
// Returns (timeMs, feedback, dampingHz).
func SendDelayParams() (float64, float64, float64) {
	if sendFX == nil || !sendFX.initialized {
		return 300, 0.3, 3000 // defaults
	}
	sendFX.mu.Lock()
	defer sendFX.mu.Unlock()
	return sendFX.delayTimeMs, sendFX.delayFeedback, sendFX.delayDampingHz
}

// SendReverbParams returns the current reverb send configuration.
// Returns (room, damping, wet).
func SendReverbParams() (float64, float64, float64) {
	if sendFX == nil || !sendFX.initialized {
		return 0.7, 0.4, 0.3 // defaults
	}
	sendFX.mu.Lock()
	defer sendFX.mu.Unlock()
	return sendFX.reverbRoom, sendFX.reverbDamping, sendFX.reverbWet
}

func clampSend(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// resetSendEffects resets both effects to silence (e.g., on stop).
func resetSendEffects() {
	if sendFX == nil || !sendFX.initialized {
		return
	}
	sendFX.mu.Lock()
	C.delay_reset(&sendFX.delay)
	C.reverb_reset(&sendFX.reverb)
	sendFX.mu.Unlock()
}
