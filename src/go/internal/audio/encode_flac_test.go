//go:build test

package audio

import (
	"bytes"
	"testing"
)

func TestFLACEncoderRegistry(t *testing.T) {
	enc, err := NewEncoder(FormatFLAC)
	if err != nil {
		t.Fatalf("NewEncoder(flac): %v", err)
	}
	if enc.Format() != FormatFLAC {
		t.Errorf("Format() = %s, want flac", enc.Format())
	}
	if enc.FileExtension() != ".flac" {
		t.Errorf("FileExtension() = %s, want .flac", enc.FileExtension())
	}
}

func TestFLACEncodeBasic(t *testing.T) {
	enc, _ := NewEncoder(FormatFLAC)

	// Simple sine-like samples
	samples := make([]float64, 4096)
	for i := range samples {
		samples[i] = float64(i%100) / 100.0 * 2.0 - 1.0
	}

	var buf bytes.Buffer
	if err := enc.Encode(&buf, samples, 44100); err != nil {
		t.Fatalf("Encode: %v", err)
	}

	data := buf.Bytes()
	// Verify fLaC marker
	if len(data) < 4 {
		t.Fatal("output too short")
	}
	if string(data[0:4]) != "fLaC" {
		t.Fatalf("missing fLaC marker, got %q", string(data[0:4]))
	}

	// Verify STREAMINFO metadata block header
	// First metadata block: last=1, type=0, length=34
	blockHeader := uint32(data[4])<<24 | uint32(data[5])<<16 | uint32(data[6])<<8 | uint32(data[7])
	isLast := (blockHeader >> 31) == 1
	blockType := (blockHeader >> 24) & 0x7F
	blockLen := blockHeader & 0xFFFFFF
	if !isLast {
		t.Error("STREAMINFO should be marked as last metadata block")
	}
	if blockType != 0 {
		t.Errorf("block type = %d, want 0 (STREAMINFO)", blockType)
	}
	if blockLen != 34 {
		t.Errorf("block length = %d, want 34", blockLen)
	}

	// Output should be non-trivially sized (at least header + some frame data)
	if len(data) < 100 {
		t.Errorf("output suspiciously small: %d bytes", len(data))
	}
}

func TestFLACEncodeEmpty(t *testing.T) {
	enc, _ := NewEncoder(FormatFLAC)
	var buf bytes.Buffer
	err := enc.Encode(&buf, nil, 44100)
	if err == nil {
		t.Error("expected error for empty samples")
	}
}

func TestFLACEncodeSmallBlock(t *testing.T) {
	enc, _ := NewEncoder(FormatFLAC)
	// Very small sample count (less than one block)
	samples := []float64{0.1, 0.2, 0.3, 0.4, 0.5}

	var buf bytes.Buffer
	if err := enc.Encode(&buf, samples, 44100); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if string(buf.Bytes()[0:4]) != "fLaC" {
		t.Fatal("missing fLaC marker for small block")
	}
}

func TestFLACEncode48kHz(t *testing.T) {
	enc, _ := NewEncoder(FormatFLAC)
	samples := make([]float64, 1024)
	for i := range samples {
		samples[i] = 0.5
	}

	var buf bytes.Buffer
	if err := enc.Encode(&buf, samples, 48000); err != nil {
		t.Fatalf("Encode at 48kHz: %v", err)
	}
	if string(buf.Bytes()[0:4]) != "fLaC" {
		t.Fatal("missing fLaC marker for 48kHz")
	}
}

func TestFlacBlockSizeCode(t *testing.T) {
	tests := []struct {
		n    int
		want int
	}{
		{192, 1},
		{576, 2},
		{1152, 3},
		{2304, 4},
		{4608, 5},
		{256, 8},
		{512, 9},
		{1024, 10},
		{2048, 11},
		{4096, 12},
		{8192, 13},
		{16384, 14},
		{32768, 15},
		{100, 6},   // 8-bit block size (<=256)
		{500, 7},   // 16-bit block size (>256, non-standard)
		{3000, 7},  // 16-bit block size
	}
	for _, tt := range tests {
		got := flacBlockSizeCode(tt.n)
		if got != tt.want {
			t.Errorf("flacBlockSizeCode(%d) = %d, want %d", tt.n, got, tt.want)
		}
	}
}

func TestFlacSampleRateCode(t *testing.T) {
	tests := []struct {
		sr   int
		want int
	}{
		{88200, 1},
		{176400, 2},
		{192000, 3},
		{8000, 4},
		{16000, 5},
		{22050, 6},
		{24000, 7},
		{32000, 8},
		{44100, 9},
		{48000, 10},
		{96000, 11},
		{12345, 12}, // non-standard → kHz
	}
	for _, tt := range tests {
		got := flacSampleRateCode(tt.sr)
		if got != tt.want {
			t.Errorf("flacSampleRateCode(%d) = %d, want %d", tt.sr, got, tt.want)
		}
	}
}

func TestFlacSampleSizeCode(t *testing.T) {
	tests := []struct {
		bps  int
		want int
	}{
		{8, 1},
		{12, 2},
		{16, 4},
		{20, 5},
		{24, 6},
		{32, 7},
		{14, 0}, // non-standard
	}
	for _, tt := range tests {
		got := flacSampleSizeCode(tt.bps)
		if got != tt.want {
			t.Errorf("flacSampleSizeCode(%d) = %d, want %d", tt.bps, got, tt.want)
		}
	}
}

func TestFlacCRC8Table(t *testing.T) {
	// CRC-8 of empty data should be 0
	var crc byte
	// Table should be initialized
	if flacCRC8Table[0] != 0 {
		t.Errorf("CRC8 table[0] = %d, want 0", flacCRC8Table[0])
	}
	// CRC-8 of 0xFF should be non-zero
	crc = flacCRC8Table[0xFF]
	if crc == 0 {
		t.Error("CRC8 of 0xFF should be non-zero")
	}
}

func TestFlacCRC16Table(t *testing.T) {
	if flacCRC16Table[0] != 0 {
		t.Errorf("CRC16 table[0] = %d, want 0", flacCRC16Table[0])
	}
	if flacCRC16Table[0xFF] == 0 {
		t.Error("CRC16 of 0xFF should be non-zero")
	}
}

func TestFlacFrameBuilderAlignByteNoop(t *testing.T) {
	var fb flacFrameBuilder
	fb.init()
	// Write exactly 8 bits → byte already aligned
	fb.writeBits(0xAB, 8)
	lenBefore := len(fb.bytes())
	fb.alignByte() // should be a no-op since bits == 0
	if len(fb.bytes()) != lenBefore {
		t.Errorf("alignByte on aligned builder added bytes: %d → %d", lenBefore, len(fb.bytes()))
	}
}

func TestFlacFrameBuilderAlignBytePartial(t *testing.T) {
	var fb flacFrameBuilder
	fb.init()
	// Write 4 bits → need alignment
	fb.writeBits(0xF, 4)
	fb.alignByte()
	data := fb.bytes()
	if len(data) != 1 {
		t.Errorf("alignByte should produce 1 byte, got %d", len(data))
	}
	// Top 4 bits should be 0xF, bottom 4 should be 0
	if data[0] != 0xF0 {
		t.Errorf("aligned byte = 0x%02X, want 0xF0", data[0])
	}
}

func TestFlacFrameBuilderWriteBits(t *testing.T) {
	var fb flacFrameBuilder
	fb.init()
	fb.writeBits(0xFF, 8)
	fb.alignByte()
	data := fb.bytes()
	if len(data) != 1 || data[0] != 0xFF {
		t.Errorf("writeBits(0xFF, 8) = %v, want [0xFF]", data)
	}
}

func TestFlacFrameBuilderWriteUTF8(t *testing.T) {
	var fb flacFrameBuilder
	fb.init()
	fb.writeUTF8(0) // single byte
	fb.alignByte()
	if len(fb.bytes()) != 1 {
		t.Errorf("UTF8(0) length = %d, want 1", len(fb.bytes()))
	}

	fb.init()
	fb.writeUTF8(0x100) // 2 bytes
	fb.alignByte()
	if len(fb.bytes()) != 2 {
		t.Errorf("UTF8(0x100) length = %d, want 2", len(fb.bytes()))
	}

	fb.init()
	fb.writeUTF8(0x1000) // 3 bytes
	fb.alignByte()
	if len(fb.bytes()) != 3 {
		t.Errorf("UTF8(0x1000) length = %d, want 3", len(fb.bytes()))
	}

	fb.init()
	fb.writeUTF8(0x10000) // 4 bytes
	fb.alignByte()
	if len(fb.bytes()) != 4 {
		t.Errorf("UTF8(0x10000) length = %d, want 4", len(fb.bytes()))
	}
}
