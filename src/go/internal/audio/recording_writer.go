package audio

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
)

// streamSink is the surface streamWavWriter needs from its output. *os.File
// satisfies it; tests can substitute an in-memory implementation.
type streamSink interface {
	io.Writer
	io.Seeker
	io.Closer
}

// streamWavWriter writes a WAV file incrementally: header up-front with
// placeholder sizes, sample frames streamed via a buffered writer, then the
// header is patched on Close. Avoids the full-buffer-in-memory cost and the
// per-sample bytes.Buffer.Write hot loop in the legacy encoder path.
//
// Designed for use from a single goroutine (one per channel). Not safe for
// concurrent Write calls.
type streamWavWriter struct {
	sink       streamSink
	buf        *bufio.Writer
	format     AudioFormat
	sampleRate int
	bps        int  // bits per sample
	isFloat    bool // true → IEEE float WAV (audio format tag 3)
	written    uint32 // bytes written to the data chunk so far
	closed     bool

	// Pre-allocated scratch for sample-block conversion. Sized once at New;
	// not grown on hot path.
	scratch []byte
}

const wavHeaderSize = 44 // RIFF + fmt + data headers, before samples

// newStreamWavWriter opens path for writing and emits the WAV header with
// placeholder size fields. The caller must call Close to patch them.
func newStreamWavWriter(path string, format AudioFormat, sampleRate int) (*streamWavWriter, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	w, err := newStreamWavWriterTo(f, format, sampleRate)
	if err != nil {
		f.Close()
		return nil, err
	}
	return w, nil
}

// newStreamWavWriterTo wraps an arbitrary streamSink. Used by tests.
func newStreamWavWriterTo(sink streamSink, format AudioFormat, sampleRate int) (*streamWavWriter, error) {
	bps, isFloat, err := wavFormatBits(format)
	if err != nil {
		return nil, err
	}
	w := &streamWavWriter{
		sink:       sink,
		buf:        bufio.NewWriterSize(sink, 16*1024),
		format:     format,
		sampleRate: sampleRate,
		bps:        bps,
		isFloat:    isFloat,
		scratch:    make([]byte, 4096),
	}
	if err := w.writeHeader(0); err != nil {
		return nil, err
	}
	return w, nil
}

func wavFormatBits(format AudioFormat) (bps int, isFloat bool, err error) {
	switch format {
	case FormatWAV16:
		return 16, false, nil
	case FormatWAV24:
		return 24, false, nil
	case FormatWAV32:
		return 32, true, nil
	default:
		return 0, false, fmt.Errorf("unsupported streaming WAV format: %s", format)
	}
}

func (w *streamWavWriter) writeHeader(dataSize uint32) error {
	const numChannels uint16 = 1
	bps := uint16(w.bps)
	bytesPerSample := bps / 8
	blockAlign := numChannels * bytesPerSample
	byteRate := uint32(w.sampleRate) * uint32(blockAlign)
	var audioFmt uint16 = 1
	if w.isFloat {
		audioFmt = 3
	}
	var hdr [wavHeaderSize]byte
	copy(hdr[0:4], "RIFF")
	binary.LittleEndian.PutUint32(hdr[4:8], 36+dataSize)
	copy(hdr[8:12], "WAVE")
	copy(hdr[12:16], "fmt ")
	binary.LittleEndian.PutUint32(hdr[16:20], 16)
	binary.LittleEndian.PutUint16(hdr[20:22], audioFmt)
	binary.LittleEndian.PutUint16(hdr[22:24], numChannels)
	binary.LittleEndian.PutUint32(hdr[24:28], uint32(w.sampleRate))
	binary.LittleEndian.PutUint32(hdr[28:32], byteRate)
	binary.LittleEndian.PutUint16(hdr[32:34], blockAlign)
	binary.LittleEndian.PutUint16(hdr[34:36], bps)
	copy(hdr[36:40], "data")
	binary.LittleEndian.PutUint32(hdr[40:44], dataSize)
	_, err := w.buf.Write(hdr[:])
	return err
}

// WriteSamplesFloat32 appends float32 samples in [-1,1] to the data chunk,
// converting on the fly. Pre-allocated scratch is reused; no per-sample
// allocation. The caller may pass any-size slice.
func (w *streamWavWriter) WriteSamplesFloat32(samples []float32) error {
	if w.closed {
		return fmt.Errorf("streamWavWriter: write after close")
	}
	bytesPerSample := w.bps / 8
	if cap(w.scratch) < len(samples)*bytesPerSample {
		w.scratch = make([]byte, len(samples)*bytesPerSample)
	}
	scratch := w.scratch[:len(samples)*bytesPerSample]
	switch w.bps {
	case 16:
		for i, s := range samples {
			s = clampFloat32(s)
			v := int16(s * 32767)
			scratch[i*2] = byte(v)
			scratch[i*2+1] = byte(v >> 8)
		}
	case 24:
		for i, s := range samples {
			s = clampFloat32(s)
			v := int32(s * 8388607)
			scratch[i*3] = byte(v)
			scratch[i*3+1] = byte(v >> 8)
			scratch[i*3+2] = byte(v >> 16)
		}
	case 32:
		for i, s := range samples {
			s = clampFloat32(s)
			binary.LittleEndian.PutUint32(scratch[i*4:], math.Float32bits(s))
		}
	}
	n, err := w.buf.Write(scratch)
	w.written += uint32(n)
	return err
}

// WriteSilence appends n samples of silence efficiently (single buffered
// write, no per-sample loop).
func (w *streamWavWriter) WriteSilence(n int) error {
	if w.closed || n <= 0 {
		return nil
	}
	bytesPerSample := w.bps / 8
	bytes := n * bytesPerSample
	// Reuse scratch as a zero buffer. Grow if needed but don't grow forever:
	// silence runs are typically a few blocks long.
	if cap(w.scratch) < bytes {
		w.scratch = make([]byte, bytes)
	} else {
		// Zero only the prefix we'll write.
		s := w.scratch[:bytes]
		for i := range s {
			s[i] = 0
		}
	}
	written, err := w.buf.Write(w.scratch[:bytes])
	w.written += uint32(written)
	return err
}

// SamplesWritten returns the number of samples written so far (excluding
// header).
func (w *streamWavWriter) SamplesWritten() int {
	bytesPerSample := w.bps / 8
	if bytesPerSample == 0 {
		return 0
	}
	return int(w.written) / bytesPerSample
}

// Close flushes the buffered writer and patches the RIFF + data size fields
// in the header. The underlying sink is closed.
func (w *streamWavWriter) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true
	if err := w.buf.Flush(); err != nil {
		_ = w.sink.Close()
		return err
	}
	dataSize := w.written
	// Patch RIFF size at offset 4.
	if _, err := w.sink.Seek(4, io.SeekStart); err != nil {
		_ = w.sink.Close()
		return err
	}
	var sizeBuf [4]byte
	binary.LittleEndian.PutUint32(sizeBuf[:], 36+dataSize)
	if _, err := w.sink.Write(sizeBuf[:]); err != nil {
		_ = w.sink.Close()
		return err
	}
	// Patch data size at offset 40.
	if _, err := w.sink.Seek(40, io.SeekStart); err != nil {
		_ = w.sink.Close()
		return err
	}
	binary.LittleEndian.PutUint32(sizeBuf[:], dataSize)
	if _, err := w.sink.Write(sizeBuf[:]); err != nil {
		_ = w.sink.Close()
		return err
	}
	return w.sink.Close()
}

func clampFloat32(s float32) float32 {
	if s > 1 {
		return 1
	}
	if s < -1 {
		return -1
	}
	return s
}
