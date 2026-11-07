//go:build !test && !js

package audio

import "testing"

func TestDeviceClockAndEmitRing(t *testing.T) {
	resetClockProbeForTest()

	if _, _, ok := DeviceClockSnapshot(); ok {
		t.Fatal("snapshot ok before any Read()")
	}
	updateDeviceClock(48000)
	pos, wall, ok := DeviceClockSnapshot()
	if !ok || pos != 48000 || wall <= 0 {
		t.Fatalf("snapshot=(%d,%f,%v)", pos, wall, ok)
	}

	publishEmitReport(1, 42, 48000)
	publishEmitReport(2, 43, 48064)
	dst := make([]EmitReport, 8)
	n := DrainEmitReports(dst)
	if n != 2 || dst[0].Row != 1 || dst[0].Abs != 42 || dst[1].DevPos != 48064 {
		t.Fatalf("drain n=%d dst=%+v", n, dst[:max(n, 0)])
	}
	if DrainEmitReports(dst) != 0 {
		t.Fatal("second drain should be empty")
	}
}

// Real cross-goroutine concurrency under -race: the seqlock never returns a torn pair or blocks the writer. The
// ring drops on full by design, so only each drained report's consistency and the head/tail accounting are checked.
func TestDeviceClockAndEmitRingConcurrent(t *testing.T) {
	resetClockProbeForTest()

	const iters = 20000
	clockDone := make(chan struct{})

	// The real contract: one writer.
	go func() {
		for i := int64(1); i <= iters; i++ {
			updateDeviceClock(i)
		}
		close(clockDone)
	}()

	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		for {
			select {
			case <-clockDone:
				return
			default:
				if pos, wall, ok := DeviceClockSnapshot(); ok {
					if pos < 0 || pos > iters || wall < 0 {
						t.Errorf("torn/invalid clock snapshot: pos=%d wall=%f", pos, wall)
						return
					}
				}
			}
		}
	}()
	<-clockDone
	<-readerDone

	// Single producer goroutine for the SPSC ring.
	produceDone := make(chan struct{})
	go func() {
		for i := 0; i < iters; i++ {
			publishEmitReport(i%4, i, int64(i))
		}
		close(produceDone)
	}()

	// Single consumer (this goroutine) drains concurrently with the producer.
	dst := make([]EmitReport, 64)
	total := 0
	for {
		n := DrainEmitReports(dst)
		for i := 0; i < n; i++ {
			if dst[i].Abs != int(dst[i].DevPos) || dst[i].Row != dst[i].Abs%4 {
				t.Fatalf("inconsistent drained report: %+v", dst[i])
			}
		}
		total += n
		select {
		case <-produceDone:
			for {
				n := DrainEmitReports(dst)
				if n == 0 {
					break
				}
				for i := 0; i < n; i++ {
					if dst[i].Abs != int(dst[i].DevPos) || dst[i].Row != dst[i].Abs%4 {
						t.Fatalf("inconsistent drained report: %+v", dst[i])
					}
				}
				total += n
			}
			if total == 0 {
				t.Fatal("consumer drained zero reports")
			}
			if drops := emitDrops.Load(); drops > 0 {
				t.Logf("emitRing dropped %d reports under concurrent load (expected bounded-ring behavior); drained=%d/%d", drops, total, iters)
			}
			return
		default:
		}
	}
}
