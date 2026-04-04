package analyzer

import "sync/atomic"

// RingBuffer is a single-producer, single-consumer (SPSC) circular buffer
// for float64 samples. It uses monotonically increasing atomic counters for
// the read and write positions, enabling lock-free operation between one
// producer goroutine and one consumer goroutine.
//
// The producer calls Write, which overwrites the oldest data on overflow
// (never blocks). The consumer calls Read, which drains available samples.
type RingBuffer struct {
	buf  []float64
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
		buf: make([]float64, capacity),
		cap: int64(capacity),
	}
}

// Write appends data to the buffer. If the incoming data exceeds the buffer
// capacity, only the last cap samples are kept. If writing would overflow
// the unread region, the read pointer is advanced so the buffer always
// contains the most recent samples. Write never blocks.
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

	// Copy data into the circular buffer using modular indexing.
	for i := int64(0); i < n; i++ {
		r.buf[(wPos+i)%cap] = data[i]
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
		dst[i] = r.buf[(rPos+i)%cap]
	}

	r.rPos.Store(rPos + n)
	return int(n)
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
