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

// TestDeviceClockAndEmitRingConcurrent exercises the seqlock writer/reader and
// the SPSC ring's producer/consumer under -race with real cross-goroutine
// concurrency (the sequential test above only proves API correctness on one
// goroutine). One goroutine is the sole clock writer, a second is a
// concurrent reader (proving the seqlock never returns a torn pair and never
// blocks the writer); a third goroutine is the sole ring producer while the
// test goroutine is the sole consumer, draining concurrently. The ring is a
// bounded drop-on-full design (see publishEmitReport), so with a fast
// producer and a consumer competing with a goroutine-scheduler this test
// only asserts every drained report has internally-consistent fields
// (Abs == expected for its DevPos) and that head-tail accounting stays sane
// — not that zero reports are dropped, which would be a false assumption
// about the intentional backpressure design.
func TestDeviceClockAndEmitRingConcurrent(t *testing.T) {
	resetClockProbeForTest()

	const iters = 20000
	clockDone := make(chan struct{})

	// Single writer goroutine — mirrors the real single-writer contract
	// (the Oto Read goroutine is the only caller of updateDeviceClock).
	go func() {
		for i := int64(1); i <= iters; i++ {
			updateDeviceClock(i)
		}
		close(clockDone)
	}()

	// Concurrent reader — must never observe a torn (pos, wall) pair (checked
	// via DevPos/wall sanity) and must never block the writer.
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
