package audio

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

// flacEncoder encodes audio samples to FLAC format using verbatim (uncompressed) frames.
// This produces valid FLAC files that any decoder can read, without requiring
// external compression libraries. File sizes are similar to 16-bit WAV.
type flacEncoder struct{}

func init() {
	RegisterEncoder(FormatFLAC, func() AudioEncoder { return &flacEncoder{} })
}

func (e *flacEncoder) Format() AudioFormat   { return FormatFLAC }
func (e *flacEncoder) FileExtension() string { return ".flac" }

func (e *flacEncoder) Encode(w io.Writer, samples []float64, sampleRate int) error {
	if len(samples) == 0 {
		return fmt.Errorf("no samples to encode")
	}

	const bitsPerSample = 16
	const numChannels = 1
	totalSamples := uint64(len(samples))

	// Convert to int16
	pcm := make([]int16, len(samples))
	for i, s := range samples {
		s = clampSample(s)
		pcm[i] = int16(s * 32767)
	}

	// fLaC marker
	if _, err := w.Write([]byte("fLaC")); err != nil {
		return err
	}

	// STREAMINFO metadata block (last=1, type=0, length=34)
	header := uint32(1<<31 | 0<<24 | 34)
	if err := writeBE(w, header); err != nil {
		return err
	}

	// STREAMINFO body (34 bytes)
	blockSize := 4096
	if len(pcm) < blockSize {
		blockSize = len(pcm)
	}
	// min/max block size (16 bits each)
	if err := writeBE(w, uint16(blockSize)); err != nil {
		return err
	}
	if err := writeBE(w, uint16(blockSize)); err != nil {
		return err
	}
	// min/max frame size (24 bits each) = 0 (unknown)
	if _, err := w.Write([]byte{0, 0, 0, 0, 0, 0}); err != nil {
		return err
	}
	// sample rate (20 bits) | channels-1 (3 bits) | bps-1 (5 bits) | total samples high 4 bits
	sr := uint32(sampleRate)
	packed := (sr << 12) | (uint32(numChannels-1) << 9) | (uint32(bitsPerSample-1) << 4) | uint32(totalSamples>>32)
	if err := writeBE(w, packed); err != nil {
		return err
	}
	// total samples low 32 bits
	if err := writeBE(w, uint32(totalSamples)); err != nil {
		return err
	}
	// MD5 signature (16 bytes) = 0 (not computed for simplicity)
	if _, err := w.Write(make([]byte, 16)); err != nil {
		return err
	}

	// Write frames
	frameNum := uint32(0)
	for offset := 0; offset < len(pcm); offset += blockSize {
		end := offset + blockSize
		if end > len(pcm) {
			end = len(pcm)
		}
		block := pcm[offset:end]
		if err := writeFlacVerbatimFrame(w, block, sampleRate, bitsPerSample, numChannels, frameNum); err != nil {
			return err
		}
		frameNum++
	}

	return nil
}

func writeFlacVerbatimFrame(w io.Writer, block []int16, sampleRate, bps, channels int, frameNum uint32) error {
	// We'll build the frame in a buffer to compute the CRC
	var fb flacFrameBuilder
	fb.init()

	// Frame header
	fb.writeBits(0x3FFE, 14) // sync code
	fb.writeBits(0, 1)       // reserved
	fb.writeBits(0, 1)       // blocking strategy (fixed)

	// Block size code
	bsCode := flacBlockSizeCode(len(block))
	fb.writeBits(uint32(bsCode), 4)

	// Sample rate code
	srCode := flacSampleRateCode(sampleRate)
	fb.writeBits(uint32(srCode), 4)

	// Channel assignment (mono = 0)
	fb.writeBits(0, 4)

	// Sample size code
	ssCode := flacSampleSizeCode(bps)
	fb.writeBits(uint32(ssCode), 3)

	fb.writeBits(0, 1) // reserved

	// Frame number (UTF-8 coded)
	fb.writeUTF8(frameNum)

	// Block size if code indicates "get from end of header"
	if bsCode == 6 {
		fb.writeBits(uint32(len(block)-1), 8)
	} else if bsCode == 7 {
		fb.writeBits(uint32(len(block)-1), 16)
	}

	// Sample rate if code indicates "get from end of header"
	if srCode == 12 {
		fb.writeBits(uint32(sampleRate/1000), 8)
	}

	// Frame header CRC-8
	fb.alignByte()
	crc8 := fb.crc8()
	fb.writeByteRaw(crc8)

	// Subframe: verbatim
	fb.writeBits(0, 1) // zero padding
	fb.writeBits(1, 6) // subframe type = verbatim (000001)
	fb.writeBits(0, 1) // no wasted bits

	// Verbatim samples
	for _, s := range block {
		fb.writeBits(uint32(uint16(s)), uint(bps))
	}

	fb.alignByte()

	// Frame footer CRC-16
	crc16 := fb.crc16()

	// Write everything
	if _, err := w.Write(fb.bytes()); err != nil {
		return err
	}
	return writeBE(w, crc16)
}

// flacFrameBuilder accumulates bits for a FLAC frame.
type flacFrameBuilder struct {
	buf    []byte
	bitBuf uint32
	bits   uint
}

func (fb *flacFrameBuilder) init() {
	fb.buf = fb.buf[:0]
	fb.bitBuf = 0
	fb.bits = 0
}

func (fb *flacFrameBuilder) writeBits(val uint32, n uint) {
	for n > 0 {
		space := 8 - fb.bits
		if n <= space {
			fb.bitBuf |= (val & ((1 << n) - 1)) << (space - n)
			fb.bits += n
			if fb.bits == 8 {
				fb.buf = append(fb.buf, byte(fb.bitBuf))
				fb.bitBuf = 0
				fb.bits = 0
			}
			return
		}
		// Fill current byte
		take := space
		fb.bitBuf |= (val >> (n - take)) & ((1 << take) - 1)
		fb.bits = 8
		fb.buf = append(fb.buf, byte(fb.bitBuf))
		fb.bitBuf = 0
		fb.bits = 0
		n -= take
	}
}

func (fb *flacFrameBuilder) alignByte() {
	if fb.bits > 0 {
		fb.buf = append(fb.buf, byte(fb.bitBuf))
		fb.bitBuf = 0
		fb.bits = 0
	}
}

func (fb *flacFrameBuilder) writeByteRaw(b byte) {
	fb.buf = append(fb.buf, b)
}

func (fb *flacFrameBuilder) writeUTF8(val uint32) {
	if val < 0x80 {
		fb.writeBits(val, 8)
	} else if val < 0x800 {
		fb.writeBits(0xC0|(val>>6), 8)
		fb.writeBits(0x80|(val&0x3F), 8)
	} else if val < 0x10000 {
		fb.writeBits(0xE0|(val>>12), 8)
		fb.writeBits(0x80|((val>>6)&0x3F), 8)
		fb.writeBits(0x80|(val&0x3F), 8)
	} else {
		fb.writeBits(0xF0|(val>>18), 8)
		fb.writeBits(0x80|((val>>12)&0x3F), 8)
		fb.writeBits(0x80|((val>>6)&0x3F), 8)
		fb.writeBits(0x80|(val&0x3F), 8)
	}
}

func (fb *flacFrameBuilder) bytes() []byte { return fb.buf }

func (fb *flacFrameBuilder) crc8() byte {
	var crc byte
	for _, b := range fb.buf {
		crc = flacCRC8Table[crc^b]
	}
	return crc
}

func (fb *flacFrameBuilder) crc16() uint16 {
	var crc uint16
	for _, b := range fb.buf {
		crc = (crc << 8) ^ flacCRC16Table[(crc>>8)^uint16(b)]
	}
	return crc
}

func flacBlockSizeCode(n int) int {
	switch n {
	case 192:
		return 1
	case 576:
		return 2
	case 1152:
		return 3
	case 2304:
		return 4
	case 4608:
		return 5
	case 256:
		return 8
	case 512:
		return 9
	case 1024:
		return 10
	case 2048:
		return 11
	case 4096:
		return 12
	case 8192:
		return 13
	case 16384:
		return 14
	case 32768:
		return 15
	}
	if n <= 256 {
		return 6 // 8-bit block size - 1
	}
	return 7 // 16-bit block size - 1
}

func flacSampleRateCode(sr int) int {
	switch sr {
	case 88200:
		return 1
	case 176400:
		return 2
	case 192000:
		return 3
	case 8000:
		return 4
	case 16000:
		return 5
	case 22050:
		return 6
	case 24000:
		return 7
	case 32000:
		return 8
	case 44100:
		return 9
	case 48000:
		return 10
	case 96000:
		return 11
	}
	return 12 // 8-bit sample rate in kHz
}

func flacSampleSizeCode(bps int) int {
	switch bps {
	case 8:
		return 1
	case 12:
		return 2
	case 16:
		return 4
	case 20:
		return 5
	case 24:
		return 6
	case 32:
		return 7 // technically not standard FLAC, but valid
	}
	return 0 // get from STREAMINFO
}

func writeBE(w io.Writer, v any) error {
	return binary.Write(w, binary.BigEndian, v)
}

// CRC-8 table for FLAC (polynomial 0x07)
var flacCRC8Table [256]byte

// CRC-16 table for FLAC (polynomial 0x8005)
var flacCRC16Table [256]uint16

func init() {
	for i := 0; i < 256; i++ {
		crc8 := byte(i)
		for j := 0; j < 8; j++ {
			if crc8&0x80 != 0 {
				crc8 = (crc8 << 1) ^ 0x07
			} else {
				crc8 <<= 1
			}
		}
		flacCRC8Table[i] = crc8

		crc16 := uint16(i) << 8
		for j := 0; j < 8; j++ {
			if crc16&0x8000 != 0 {
				crc16 = (crc16 << 1) ^ 0x8005
			} else {
				crc16 <<= 1
			}
		}
		flacCRC16Table[i] = crc16
	}
}

// Silence the math import (used for clampSample in encode_wav.go, shared package).
var _ = math.Float32bits
