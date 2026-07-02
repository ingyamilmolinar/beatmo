//go:build test

// Package audio — pure-Go WAV decoder stub for the test build.
// Under -tags test the CGo loadAudio / miniaudio path is absent; this file
// provides a stdlib-only DecodeWAVToPCM so synthmatch/refs_smoke_test.go can
// read real 16-bit mono WAVs produced by scripts/aiff_to_wav.py.
package audio

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
)

// DecodeWAVToPCM decodes a mono or stereo PCM WAV file to mono float32 at its
// native sample rate. No resampling is applied; the caller receives the file's
// own sample rate. Supported formats: PCM (format 1) with 16-bit or 24-bit
// depth, and IEEE float (format 3) with 32-bit depth. Stereo is averaged to
// mono.
func DecodeWAVToPCM(path string) ([]float32, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, fmt.Errorf("DecodeWAVToPCM: open %q: %w", path, err)
	}
	defer f.Close()

	// Read RIFF header.
	var riffID [4]byte
	if _, err := io.ReadFull(f, riffID[:]); err != nil {
		return nil, 0, fmt.Errorf("DecodeWAVToPCM: read RIFF id: %w", err)
	}
	if string(riffID[:]) != "RIFF" {
		return nil, 0, fmt.Errorf("DecodeWAVToPCM: not a RIFF file")
	}
	var chunkSize uint32
	if err := binary.Read(f, binary.LittleEndian, &chunkSize); err != nil {
		return nil, 0, fmt.Errorf("DecodeWAVToPCM: read chunk size: %w", err)
	}
	var waveID [4]byte
	if _, err := io.ReadFull(f, waveID[:]); err != nil {
		return nil, 0, fmt.Errorf("DecodeWAVToPCM: read WAVE id: %w", err)
	}
	if string(waveID[:]) != "WAVE" {
		return nil, 0, fmt.Errorf("DecodeWAVToPCM: not a WAVE file")
	}

	// Walk sub-chunks to find "fmt " and "data".
	var (
		sampleRate  int
		numChannels int
		bitsPerSamp int
		audioFmt    uint16
		dataPayload []byte
	)
	for {
		var id [4]byte
		if _, err := io.ReadFull(f, id[:]); err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		} else if err != nil {
			return nil, 0, fmt.Errorf("DecodeWAVToPCM: read chunk id: %w", err)
		}
		var sz uint32
		if err := binary.Read(f, binary.LittleEndian, &sz); err != nil {
			return nil, 0, fmt.Errorf("DecodeWAVToPCM: read chunk sz: %w", err)
		}
		switch string(id[:]) {
		case "fmt ":
			if sz < 16 {
				return nil, 0, fmt.Errorf("DecodeWAVToPCM: fmt chunk too small (%d)", sz)
			}
			buf := make([]byte, sz)
			if _, err := io.ReadFull(f, buf); err != nil {
				return nil, 0, fmt.Errorf("DecodeWAVToPCM: read fmt chunk: %w", err)
			}
			audioFmt = binary.LittleEndian.Uint16(buf[0:2])
			numChannels = int(binary.LittleEndian.Uint16(buf[2:4]))
			sampleRate = int(binary.LittleEndian.Uint32(buf[4:8]))
			bitsPerSamp = int(binary.LittleEndian.Uint16(buf[14:16]))
		case "data":
			dataPayload = make([]byte, sz)
			if _, err := io.ReadFull(f, dataPayload); err != nil {
				return nil, 0, fmt.Errorf("DecodeWAVToPCM: read data chunk: %w", err)
			}
		default:
			// Skip unknown chunks.
			if _, err := f.Seek(int64(sz), io.SeekCurrent); err != nil {
				return nil, 0, fmt.Errorf("DecodeWAVToPCM: seek past chunk %q: %w", string(id[:]), err)
			}
		}
	}

	// audioFmt 1 = PCM (16-bit or 24-bit), 3 = IEEE float (32-bit).
	if audioFmt != 1 && audioFmt != 3 {
		return nil, 0, fmt.Errorf("DecodeWAVToPCM: unsupported audio format %d (only PCM=1, IEEE float=3)", audioFmt)
	}
	if numChannels < 1 || numChannels > 2 {
		return nil, 0, fmt.Errorf("DecodeWAVToPCM: unsupported channel count %d", numChannels)
	}
	if audioFmt == 1 && bitsPerSamp != 16 && bitsPerSamp != 24 {
		return nil, 0, fmt.Errorf("DecodeWAVToPCM: unsupported bit depth %d for PCM (only 16 or 24)", bitsPerSamp)
	}
	if audioFmt == 3 && bitsPerSamp != 32 {
		return nil, 0, fmt.Errorf("DecodeWAVToPCM: unsupported bit depth %d for IEEE float (only 32)", bitsPerSamp)
	}
	if sampleRate <= 0 {
		return nil, 0, fmt.Errorf("DecodeWAVToPCM: invalid sample rate %d", sampleRate)
	}
	if len(dataPayload) == 0 {
		return nil, 0, fmt.Errorf("DecodeWAVToPCM: no data chunk found")
	}

	bytesPerSample := bitsPerSamp / 8
	bytesPerFrame := numChannels * bytesPerSample
	nFrames := len(dataPayload) / bytesPerFrame
	out := make([]float32, nFrames)
	for i := range out {
		base := i * bytesPerFrame
		var sum float64
		for ch := 0; ch < numChannels; ch++ {
			off := base + ch*bytesPerSample
			switch {
			case audioFmt == 1 && bitsPerSamp == 16:
				s := int16(binary.LittleEndian.Uint16(dataPayload[off:]))
				sum += float64(s) / 32768.0
			case audioFmt == 1 && bitsPerSamp == 24:
				b0, b1, b2 := dataPayload[off], dataPayload[off+1], dataPayload[off+2]
				raw := int32(b0) | int32(b1)<<8 | int32(b2)<<16
				if raw&0x800000 != 0 {
					raw -= 0x1000000
				}
				sum += float64(raw) / 8388608.0
			case audioFmt == 3 && bitsPerSamp == 32:
				bits := binary.LittleEndian.Uint32(dataPayload[off:])
				sum += float64(math.Float32frombits(bits))
			}
		}
		out[i] = float32(sum / float64(numChannels))
	}
	return out, sampleRate, nil
}
