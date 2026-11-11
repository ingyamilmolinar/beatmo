package eventlogger

import (
	"bytes"
	"sync"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/async"
	"go.uber.org/goleak"
)

// syncBuffer is a concurrency-safe capture buffer for tests. The eventlogger
// emits on the coalescer's AfterFunc timer goroutines and its drain goroutine,
// so a test reading a raw bytes.Buffer while a straggler line is still being
// written races the writer (a wall-clock sleep is not a happens-before edge).
// Routing both writes and reads through one mutex removes that race.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// TestMain enables goleak verification. Pre-allocate the standing pool so
// its worker goroutine exists before IgnoreCurrent snapshots the baseline;
// tests that legitimately leak (Loggers not Closed) will still trip.
func TestMain(m *testing.M) {
	_, _ = async.DefaultRegistry().Get("eventlogger.format", async.Options{
		MaxConcurrent: 1, QueueSize: 8, Name: "eventlogger.format",
	})
	goleak.VerifyTestMain(m, goleak.IgnoreCurrent())
}
