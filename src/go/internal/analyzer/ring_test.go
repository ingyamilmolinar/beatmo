package analyzer

import (
	"runtime"
	"sync/atomic"
	"testing"
)

func TestWriteThenRead(t *testing.T) {
	r := NewRingBuffer(8)
	r.Write([]float64{1, 2, 3, 4})
	dst := make([]float64, 8)
	n := r.Read(dst)
	if n != 4 {
		t.Fatalf("expected 4 samples, got %d", n)
	}
	want := []float64{1, 2, 3, 4}
	for i, v := range want {
		if dst[i] != v {
			t.Errorf("dst[%d] = %f, want %f", i, dst[i], v)
		}
	}
}

func TestWrapAround(t *testing.T) {
	r := NewRingBuffer(8)
	// Write 3, read 3 to advance pointers
	r.Write([]float64{1, 2, 3})
	dst := make([]float64, 8)
	n := r.Read(dst)
	if n != 3 {
		t.Fatalf("first read: expected 3, got %d", n)
	}
	// Write 3 more — this wraps around the internal buffer if pointers advanced
	r.Write([]float64{4, 5, 6})
	n = r.Read(dst)
	if n != 3 {
		t.Fatalf("second read: expected 3, got %d", n)
	}
	want := []float64{4, 5, 6}
	for i, v := range want {
		if dst[i] != v {
			t.Errorf("dst[%d] = %f, want %f", i, dst[i], v)
		}
	}
}

func TestOverflow(t *testing.T) {
	r := NewRingBuffer(4)
	// Write 6 samples into a buffer of capacity 4 — oldest 2 should be lost
	r.Write([]float64{1, 2, 3, 4, 5, 6})
	dst := make([]float64, 8)
	n := r.Read(dst)
	if n != 4 {
		t.Fatalf("expected 4 samples, got %d", n)
	}
	want := []float64{3, 4, 5, 6}
	for i, v := range want {
		if dst[i] != v {
			t.Errorf("dst[%d] = %f, want %f", i, dst[i], v)
		}
	}
}

func TestEmptyRead(t *testing.T) {
	r := NewRingBuffer(8)
	dst := make([]float64, 8)
	n := r.Read(dst)
	if n != 0 {
		t.Fatalf("expected 0 from empty buffer, got %d", n)
	}
}

func TestAvailable(t *testing.T) {
	r := NewRingBuffer(8)
	if a := r.Available(); a != 0 {
		t.Fatalf("empty: expected 0, got %d", a)
	}
	r.Write([]float64{1, 2, 3})
	if a := r.Available(); a != 3 {
		t.Fatalf("after write 3: expected 3, got %d", a)
	}
	dst := make([]float64, 2)
	r.Read(dst)
	if a := r.Available(); a != 1 {
		t.Fatalf("after read 2: expected 1, got %d", a)
	}
}

func TestConcurrentSPSC(t *testing.T) {
	const total = 10000
	const capacity = 256
	r := NewRingBuffer(capacity)

	var producerDone atomic.Bool
	var totalRead atomic.Int64

	// Producer: write values 0..9999 in chunks
	go func() {
		buf := make([]float64, 64)
		for i := 0; i < total; i += len(buf) {
			end := i + len(buf)
			if end > total {
				end = total
			}
			for j := i; j < end; j++ {
				buf[j-i] = float64(j)
			}
			r.Write(buf[:end-i])
			// Yield to give consumer a chance to run
			runtime.Gosched()
		}
		producerDone.Store(true)
	}()

	// Consumer: read until producer is done and buffer is drained.
	// Note: with overflow, values within a chunk may not be monotonic
	// because the producer can overwrite the oldest data. We just verify
	// that data is being consumed without panics or data races.
	dst := make([]float64, 128)
	for {
		n := r.Read(dst)
		if n > 0 {
			totalRead.Add(int64(n))
		} else if producerDone.Load() {
			break
		} else {
			runtime.Gosched()
		}
	}

	got := totalRead.Load()
	// This is a lossy, non-blocking overflow ring: Write never blocks and
	// overwrites the oldest unread data, so a consumer that loses the race
	// legitimately drops most samples (RingBuffer.Write doc). Under scheduler
	// starvation — e.g. this test running inside the full `make test-real`
	// suite where hundreds of goroutines contend — the producer can outrun the
	// consumer entirely; the only guaranteed floor is one full buffer drained
	// after the producer finishes. Asserting a keep-pace threshold (total/4)
	// made this test flaky (observed 576 reads under load). Assert the real
	// contract: the buffer is drainable and data flows.
	if got < capacity {
		t.Fatalf("consumer read %d samples, expected at least one full buffer (%d)", got, capacity)
	}
	t.Logf("consumer read %d / %d samples", got, total)
}
