// Package wavgen makes and parses WAV for tests and cmd/wavgen, so nothing depends on real audio. Encoding uses the
// app's recording encoder; decoding is a small pure-Go RIFF parser (the desktop decodes through C miniaudio, absent under
// -tags test).
package wavgen

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// Deterministic, so a content hash of the bytes pins a file version.
func EncodeWAV(pcm []float32, sampleRate int) ([]byte, error) {
	enc, err := audio.NewEncoder(audio.FormatWAV24)
	if err != nil {
		return nil, err
	}
	f64 := make([]float64, len(pcm))
	for i, s := range pcm {
		f64[i] = float64(s)
	}
	var buf bytes.Buffer
	if err := enc.Encode(&buf, f64, sampleRate); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// DecodeWAV parses RIFF/WAVE bytes into mono float32 PCM + sample rate. Supports
// PCM 8/16/24/32-bit and IEEE float32; multi-channel audio is downmixed to mono
// by averaging. Enough to decode our own encoder's output and typical WAV files.
func DecodeWAV(data []byte) ([]float32, int, error) {
	if len(data) < 12 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, 0, fmt.Errorf("wavgen: not a RIFF/WAVE file")
	}
	var (
		audioFormat   uint16
		numChannels   uint16
		sampleRate    uint32
		bitsPerSample uint16
		haveFmt       bool
		dataChunk     []byte
	)
	pos := 12
	for pos+8 <= len(data) {
		id := string(data[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		body := pos + 8
		if body+size > len(data) {
			size = len(data) - body // tolerate a truncated final chunk
		}
		switch id {
		case "fmt ":
			if size < 16 {
				return nil, 0, fmt.Errorf("wavgen: short fmt chunk")
			}
			audioFormat = binary.LittleEndian.Uint16(data[body : body+2])
			numChannels = binary.LittleEndian.Uint16(data[body+2 : body+4])
			sampleRate = binary.LittleEndian.Uint32(data[body+4 : body+8])
			bitsPerSample = binary.LittleEndian.Uint16(data[body+14 : body+16])
			haveFmt = true
		case "data":
			dataChunk = data[body : body+size]
		}
		pos = body + size
		if size%2 == 1 {
			pos++ // chunks are word-aligned
		}
	}
	if !haveFmt {
		return nil, 0, fmt.Errorf("wavgen: missing fmt chunk")
	}
	if numChannels == 0 {
		return nil, 0, fmt.Errorf("wavgen: zero channels")
	}
	samples, err := decodeSamples(dataChunk, audioFormat, bitsPerSample)
	if err != nil {
		return nil, 0, err
	}
	mono := downmix(samples, int(numChannels))
	return mono, int(sampleRate), nil
}

func decodeSamples(data []byte, format, bits uint16) ([]float32, error) {
	switch {
	case format == 3 && bits == 32: // IEEE float
		n := len(data) / 4
		out := make([]float32, n)
		for i := 0; i < n; i++ {
			out[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4 : i*4+4]))
		}
		return out, nil
	case format == 1 && bits == 8: // unsigned 8-bit
		out := make([]float32, len(data))
		for i, b := range data {
			out[i] = (float32(b) - 128) / 128
		}
		return out, nil
	case format == 1 && bits == 16:
		n := len(data) / 2
		out := make([]float32, n)
		for i := 0; i < n; i++ {
			out[i] = float32(int16(binary.LittleEndian.Uint16(data[i*2:i*2+2]))) / 32768
		}
		return out, nil
	case format == 1 && bits == 24:
		n := len(data) / 3
		out := make([]float32, n)
		for i := 0; i < n; i++ {
			b := data[i*3 : i*3+3]
			v := int32(b[0]) | int32(b[1])<<8 | int32(b[2])<<16
			if v&0x800000 != 0 {
				v |= ^0xFFFFFF // sign-extend
			}
			out[i] = float32(v) / 8388608
		}
		return out, nil
	case format == 1 && bits == 32:
		n := len(data) / 4
		out := make([]float32, n)
		for i := 0; i < n; i++ {
			out[i] = float32(int32(binary.LittleEndian.Uint32(data[i*4:i*4+4]))) / 2147483648
		}
		return out, nil
	}
	return nil, fmt.Errorf("wavgen: unsupported WAV format=%d bits=%d", format, bits)
}

func downmix(interleaved []float32, channels int) []float32 {
	if channels <= 1 {
		return interleaved
	}
	frames := len(interleaved) / channels
	out := make([]float32, frames)
	for f := 0; f < frames; f++ {
		var sum float32
		for c := 0; c < channels; c++ {
			sum += interleaved[f*channels+c]
		}
		out[f] = sum / float32(channels)
	}
	return out
}
