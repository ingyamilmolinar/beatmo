package eventlogger

import (
	"bytes"
	"sync"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/async"
	"go.uber.org/goleak"
)

// Lines are written from timer and drain goroutines, and a sleep is no happens-before edge, so reads and writes share a
// mutex.
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

// The standing pool's worker exists before IgnoreCurrent snapshots the baseline; an unclosed Logger still trips it.
func TestMain(m *testing.M) {
	_, _ = async.DefaultRegistry().Get("eventlogger.format", async.Options{
		MaxConcurrent: 1, QueueSize: 8, Name: "eventlogger.format",
	})
	goleak.VerifyTestMain(m, goleak.IgnoreCurrent())
}
