package audio

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
)

// memSink is an in-memory streamSink for tests. Tracks all writes and
// supports Seek for header patching.
type memSink struct {
	buf    []byte
	pos    int64
	closed bool
}

func (m *memSink) Write(p []byte) (int, error) {
	end := int(m.pos) + len(p)
	if end > len(m.buf) {
		grown := make([]byte, end)
		copy(grown, m.buf)
		m.buf = grown
	}
	copy(m.buf[m.pos:], p)
	m.pos = int64(end)
	return len(p), nil
}

func (m *memSink) Seek(off int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		m.pos = off
	case io.SeekCurrent:
		m.pos += off
	case io.SeekEnd:
		m.pos = int64(len(m.buf)) + off
	}
	return m.pos, nil
}

func (m *memSink) Close() error { m.closed = true; return nil }

func TestStreamWav_WAV24_HeaderAndSamples(t *testing.T) {
	sink := &memSink{}
	w, err := newStreamWavWriterTo(sink, FormatWAV24, 44100)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	samples := []float32{0, 0.5, -0.5, 1, -1}
	if err := w.WriteSamplesFloat32(samples); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if !sink.closed {
		t.Fatal("sink not closed")
	}
	if !bytes.Equal(sink.buf[0:4], []byte("RIFF")) {
		t.Fatal("missing RIFF")
	}
	if !bytes.Equal(sink.buf[8:12], []byte("WAVE")) {
		t.Fatal("missing WAVE")
	}
	dataSize := binary.LittleEndian.Uint32(sink.buf[40:44])
	wantData := uint32(len(samples) * 3)
	if dataSize != wantData {
		t.Fatalf("data size = %d, want %d", dataSize, wantData)
	}
	riffSize := binary.LittleEndian.Uint32(sink.buf[4:8])
	if riffSize != 36+wantData {
		t.Fatalf("riff size = %d, want %d", riffSize, 36+wantData)
	}
	bps := binary.LittleEndian.Uint16(sink.buf[34:36])
	if bps != 24 {
		t.Fatalf("bps = %d, want 24", bps)
	}
}

func TestStreamWav_WAV16_AndWAV32Float(t *testing.T) {
	for _, format := range []AudioFormat{FormatWAV16, FormatWAV32} {
		sink := &memSink{}
		w, err := newStreamWavWriterTo(sink, format, 48000)
		if err != nil {
			t.Fatalf("new(%s): %v", format, err)
		}
		if err := w.WriteSamplesFloat32([]float32{0.25, -0.25, 0, 0.99}); err != nil {
			t.Fatalf("write(%s): %v", format, err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("close(%s): %v", format, err)
		}
		audioFmt := binary.LittleEndian.Uint16(sink.buf[20:22])
		bps := binary.LittleEndian.Uint16(sink.buf[34:36])
		switch format {
		case FormatWAV16:
			if audioFmt != 1 || bps != 16 {
				t.Fatalf("WAV16 hdr: fmt=%d bps=%d", audioFmt, bps)
			}
		case FormatWAV32:
			if audioFmt != 3 || bps != 32 {
				t.Fatalf("WAV32f hdr: fmt=%d bps=%d", audioFmt, bps)
			}
		}
	}
}

func TestStreamWav_Silence(t *testing.T) {
	sink := &memSink{}
	w, err := newStreamWavWriterTo(sink, FormatWAV16, 44100)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if err := w.WriteSilence(100); err != nil {
		t.Fatalf("silence: %v", err)
	}
	if err := w.WriteSamplesFloat32([]float32{0.5}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if got := w.SamplesWritten(); got != 101 {
		t.Fatalf("SamplesWritten = %d, want 101", got)
	}
	dataSize := binary.LittleEndian.Uint32(sink.buf[40:44])
	if dataSize != 101*2 {
		t.Fatalf("data size = %d, want %d", dataSize, 101*2)
	}
	for i, b := range sink.buf[wavHeaderSize : wavHeaderSize+200] {
		if b != 0 {
			t.Fatalf("silence corrupted at byte %d: %x", i, b)
		}
	}
}

func TestStreamWav_NoAllocOnHotWritePath(t *testing.T) {
	sink := &memSink{}
	w, err := newStreamWavWriterTo(sink, FormatWAV24, 44100)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	defer w.Close()
	block := make([]float32, 512)
	// Warm scratch.
	if err := w.WriteSamplesFloat32(block); err != nil {
		t.Fatalf("warm: %v", err)
	}
	allocs := testing.AllocsPerRun(200, func() {
		_ = w.WriteSamplesFloat32(block)
	})
	if allocs != 0 {
		t.Fatalf("WriteSamplesFloat32 allocates %.2f times per call; want 0", allocs)
	}
}
