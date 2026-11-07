package analyzer

import (
	"math"
	"sync/atomic"
)

// A lock-free SPSC ring on monotonic atomic counters. Write overwrites the oldest data and never blocks, so it can clobber
// a slot the consumer is reading: each slot is an atomic.Uint64, which keeps that overlap well-defined (the consumer may
// see a newer value there, which the contract allows).
type RingBuffer struct {
	buf  []atomic.Uint64
	cap  int64
	wPos atomic.Int64 // monotonic write position (total samples written)
	rPos atomic.Int64 // monotonic read position (total samples consumed)
}

// NewRingBuffer creates a ring buffer that can hold up to capacity samples.
// Capacity must be at least 1.
func NewRingBuffer(capacity int) *RingBuffer {
	if capacity < 1 {
		capacity = 1
	}
	return &RingBuffer{
		buf: make([]atomic.Uint64, capacity),
		cap: int64(capacity),
	}
}

// Never blocks: on overflow the read pointer advances, so the buffer always holds the most recent samples.
func (r *RingBuffer) Write(data []float64) {
	n := int64(len(data))
	if n == 0 {
		return
	}

	cap := r.cap

	// If the incoming data is larger than capacity, only write the tail.
	if n > cap {
		data = data[n-cap:]
		n = cap
	}

	wPos := r.wPos.Load()

	// Atomic per slot, so an overflow clobbering a slot mid-read is well-defined, not a data race.
	for i := int64(0); i < n; i++ {
		r.buf[(wPos+i)%cap].Store(math.Float64bits(data[i]))
	}

	newWPos := wPos + n

	// If the write would overflow past the read pointer, advance the read
	// pointer so that the buffer contains exactly cap samples (the newest).
	rPos := r.rPos.Load()
	if newWPos-rPos > cap {
		r.rPos.Store(newWPos - cap)
	}

	r.wPos.Store(newWPos)
}

// Read copies up to len(dst) available samples into dst and returns the
// number of samples copied. The read pointer is advanced accordingly.
func (r *RingBuffer) Read(dst []float64) int {
	rPos := r.rPos.Load()
	wPos := r.wPos.Load()

	avail := wPos - rPos
	if avail <= 0 {
		return 0
	}

	n := int64(len(dst))
	if n > avail {
		n = avail
	}

	cap := r.cap
	for i := int64(0); i < n; i++ {
		dst[i] = math.Float64frombits(r.buf[(rPos+i)%cap].Load())
	}

	r.rPos.Store(rPos + n)
	return int(n)
}

// Consumer side only. On circuit load, so pre-load samples never produce one last non-flat tick.
func (r *RingBuffer) Reset() {
	r.rPos.Store(r.wPos.Load())
}

// Available returns the number of unread samples currently in the buffer.
func (r *RingBuffer) Available() int {
	rPos := r.rPos.Load()
	wPos := r.wPos.Load()
	avail := wPos - rPos
	if avail < 0 {
		avail = 0
	}
	return int(avail)
}
