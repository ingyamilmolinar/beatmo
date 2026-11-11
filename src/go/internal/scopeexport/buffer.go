package scopeexport

import (
	"encoding/json"
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// bufferMaxBytes caps the in-memory JSONL buffer size to avoid unbounded growth
// on long WASM sessions. When exceeded, the oldest bytes are trimmed.
const bufferMaxBytes = 4 * 1024 * 1024 // 4 MiB — ≈ hours of 2s snapshots

// serviceBuffer holds the in-memory JSONL buffer used on platforms (WASM) where
// file output isn't meaningful. Declared on the Service package so the fields
// live alongside the rest of Service's state without split-file pitfalls.
type serviceBuffer struct {
	mu sync.Mutex
	b  []byte
}

func (sb *serviceBuffer) appendLine(line []byte) {
	sb.mu.Lock()
	sb.b = append(sb.b, line...)
	if len(sb.b) > bufferMaxBytes {
		sb.b = append([]byte(nil), sb.b[len(sb.b)-bufferMaxBytes:]...)
	}
	sb.mu.Unlock()
}

func (sb *serviceBuffer) drain() []byte {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	if len(sb.b) == 0 {
		return nil
	}
	out := make([]byte, len(sb.b))
	copy(out, sb.b)
	sb.b = sb.b[:0]
	return out
}

// BufferSnapshot builds one snapshot and appends the JSONL-encoded line to the
// service's in-memory buffer. Used on WASM where the desktop's ticker+file
// writer path isn't available. Safe to call concurrently.
func (s *Service) BufferSnapshot() {
	peakObs := wave.NewPeakRMSObserver()
	fftObs := wave.NewFFTObserver(s.cfg.FFTSize)
	zcObs := wave.NewZeroCrossingObserver()

	s.tickNum++
	snap := s.buildSnapshot(peakObs, fftObs, zcObs)
	line, err := json.Marshal(snap)
	if err != nil {
		return
	}
	line = append(line, '\n')
	s.buffer.appendLine(line)
}

// DumpBuffer returns the in-memory JSONL buffer and resets it. Returns nil if
// the buffer is empty. The caller typically hands the bytes to a Blob download
// in JavaScript.
func (s *Service) DumpBuffer() []byte {
	return s.buffer.drain()
}

// BufferLen returns the number of buffered JSONL bytes without draining.
// Used by browser-test harnesses to detect snapshot activity.
func (s *Service) BufferLen() int {
	s.buffer.mu.Lock()
	defer s.buffer.mu.Unlock()
	return len(s.buffer.b)
}
