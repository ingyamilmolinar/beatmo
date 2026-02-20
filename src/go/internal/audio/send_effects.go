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
	delayBuf  unsafe.Pointer
	reverbBuf unsafe.Pointer

	// Go-side processing buffers.
	delaySendBuf  []float64 // accumulated send to delay
	reverbSendBuf []float64 // accumulated send to reverb
	delayOutBuf   []float32 // C delay output (float32)
	reverbInBuf   []float32 // C reverb input (float32)
	reverbOutBuf  []float32 // C reverb output (float32)

	sr          int
	initialized bool
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

// initSendEffects creates and initializes the global send effects.
func initSendEffects(sr int) {
	fx := &sendEffects{sr: sr}

	// Delay: 300ms delay line, 0.3 feedback, 3kHz LP damping.
	delaySamples := sr * 300 / 1000 // 300ms
	if delaySamples < 1 {
		delaySamples = 1
	}
	fx.delayBuf = C.malloc(C.size_t(delaySamples) * C.size_t(unsafe.Sizeof(C.float(0))))
	C.delay_init(&fx.delay, (*C.float)(fx.delayBuf), C.int(delaySamples),
		C.float(0.3), C.float(3000), C.int(sr))

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
