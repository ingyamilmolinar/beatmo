//go:build !test && !js

package audio

import (
	"sync/atomic"
	"time"
)

// One writer (the Oto Read goroutine) publishes (pos, wallNano); readers get a torn-free pair through an even/odd
// version counter.
var (
	dcVersion atomic.Uint64
	dcPos     atomic.Int64
	dcWall    atomic.Int64 // UnixNano
)

func updateDeviceClock(pos int64) {
	dcVersion.Add(1) // odd → write in progress
	dcPos.Store(pos)
	dcWall.Store(time.Now().UnixNano())
	dcVersion.Add(1) // even → stable
}

// wallSeconds shares Now()'s base.
func DeviceClockSnapshot() (int64, float64, bool) {
	for {
		v1 := dcVersion.Load()
		if v1 == 0 {
			return 0, 0, false
		}
		if v1&1 != 0 {
			continue // write in progress
		}
		pos := dcPos.Load()
		wall := dcWall.Load()
		if dcVersion.Load() != v1 {
			continue // torn; retry
		}
		return pos, time.Duration(wall - start.UnixNano()).Seconds(), true
	}
}

// --- Emission-report SPSC ring --------------------------------------------
type EmitReport struct {
	Row, Abs int
	DevPos   int64
}

const emitRingSize = 1024 // power of two

var (
	emitRing  [emitRingSize]EmitReport
	emitHead  atomic.Uint64 // producer (processBlock)
	emitTail  atomic.Uint64 // consumer (audio loop)
	emitDrops atomic.Uint64
)

// publishEmitReport is lock-free, non-blocking, single-producer. Drops on full.
func publishEmitReport(row, abs int, devPos int64) {
	h := emitHead.Load()
	t := emitTail.Load()
	if h-t >= emitRingSize {
		emitDrops.Add(1)
		return
	}
	emitRing[h&(emitRingSize-1)] = EmitReport{Row: row, Abs: abs, DevPos: devPos}
	emitHead.Store(h + 1) // publish after the slot write
}

// DrainEmitReports copies up to len(dst) reports; single-consumer.
func DrainEmitReports(dst []EmitReport) int {
	h := emitHead.Load()
	t := emitTail.Load()
	n := 0
	for t < h && n < len(dst) {
		dst[n] = emitRing[t&(emitRingSize-1)]
		t++
		n++
	}
	emitTail.Store(t)
	return n
}

func resetClockProbeForTest() {
	dcVersion.Store(0)
	dcPos.Store(0)
	dcWall.Store(0)
	emitHead.Store(0)
	emitTail.Store(0)
	emitDrops.Store(0)
}
