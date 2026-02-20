package audio

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

// wavEncoder encodes audio samples to WAV format with configurable bit depth.
type wavEncoder struct {
	format        AudioFormat
	bitsPerSample int
	isFloat       bool // true for 32-bit float WAV
}

func init() {
	RegisterEncoder(FormatWAV16, func() AudioEncoder {
		return &wavEncoder{format: FormatWAV16, bitsPerSample: 16}
	})
	RegisterEncoder(FormatWAV24, func() AudioEncoder {
		return &wavEncoder{format: FormatWAV24, bitsPerSample: 24}
	})
	RegisterEncoder(FormatWAV32, func() AudioEncoder {
		return &wavEncoder{format: FormatWAV32, bitsPerSample: 32, isFloat: true}
	})
}

func (e *wavEncoder) Format() AudioFormat   { return e.format }
func (e *wavEncoder) FileExtension() string { return ".wav" }

func (e *wavEncoder) Encode(w io.Writer, samples []float64, sampleRate int) error {
	if len(samples) == 0 {
		return fmt.Errorf("no samples to encode")
	}

	numChannels := uint16(1) // mono
	bps := uint16(e.bitsPerSample)
	bytesPerSample := bps / 8
	blockAlign := numChannels * bytesPerSample
	byteRate := uint32(sampleRate) * uint32(blockAlign)
	dataSize := uint32(len(samples)) * uint32(bytesPerSample)

	// Audio format tag: 1 = PCM, 3 = IEEE float
	var audioFmt uint16 = 1
	if e.isFloat {
		audioFmt = 3
	}

	// RIFF header
	if err := writeBytes(w, []byte("RIFF")); err != nil {
		return err
	}
	if err := writeLE(w, uint32(36+dataSize)); err != nil {
		return err
	}
	if err := writeBytes(w, []byte("WAVE")); err != nil {
		return err
	}

	// fmt chunk
	if err := writeBytes(w, []byte("fmt ")); err != nil {
		return err
	}
	if err := writeLE(w, uint32(16)); err != nil { // chunk size
		return err
	}
	if err := writeLE(w, audioFmt); err != nil {
		return err
	}
	if err := writeLE(w, numChannels); err != nil {
		return err
	}
	if err := writeLE(w, uint32(sampleRate)); err != nil {
		return err
	}
	if err := writeLE(w, byteRate); err != nil {
		return err
	}
	if err := writeLE(w, blockAlign); err != nil {
		return err
	}
	if err := writeLE(w, bps); err != nil {
		return err
	}

	// data chunk
	if err := writeBytes(w, []byte("data")); err != nil {
		return err
	}
	if err := writeLE(w, dataSize); err != nil {
		return err
	}

	// Write samples
	switch e.bitsPerSample {
	case 16:
		return e.writeSamples16(w, samples)
	case 24:
		return e.writeSamples24(w, samples)
	case 32:
		return e.writeSamples32Float(w, samples)
	default:
		return fmt.Errorf("unsupported bit depth: %d", e.bitsPerSample)
	}
}

func (e *wavEncoder) writeSamples16(w io.Writer, samples []float64) error {
	buf := make([]byte, 2)
	for _, s := range samples {
		s = clampSample(s)
		v := int16(s * 32767)
		binary.LittleEndian.PutUint16(buf, uint16(v))
		if _, err := w.Write(buf); err != nil {
			return err
		}
	}
	return nil
}

func (e *wavEncoder) writeSamples24(w io.Writer, samples []float64) error {
	buf := make([]byte, 3)
	for _, s := range samples {
		s = clampSample(s)
		// 24-bit signed integer: range [-8388608, 8388607]
		v := int32(s * 8388607)
		buf[0] = byte(v)
		buf[1] = byte(v >> 8)
		buf[2] = byte(v >> 16)
		if _, err := w.Write(buf); err != nil {
			return err
		}
	}
	return nil
}

func (e *wavEncoder) writeSamples32Float(w io.Writer, samples []float64) error {
	buf := make([]byte, 4)
	for _, s := range samples {
		s = clampSample(s)
		bits := math.Float32bits(float32(s))
		binary.LittleEndian.PutUint32(buf, bits)
		if _, err := w.Write(buf); err != nil {
			return err
		}
	}
	return nil
}

func clampSample(s float64) float64 {
	if s > 1 {
		return 1
	}
	if s < -1 {
		return -1
	}
	return s
}

func writeBytes(w io.Writer, b []byte) error {
	_, err := w.Write(b)
	return err
}

func writeLE(w io.Writer, v interface{}) error {
	return binary.Write(w, binary.LittleEndian, v)
}
