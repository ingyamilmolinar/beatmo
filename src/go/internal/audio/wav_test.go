//go:build !test

package audio

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func writeTestWAV(path string) error {
	const sampleRate = 44100
	samples := sampleRate / 100 // 10ms
	data := make([]int16, samples)
	for i := range data {
		data[i] = int16(math.Sin(2*math.Pi*float64(i)/float64(samples)) * 30000)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	dataSize := uint32(len(data) * 2)
	if _, err := f.Write([]byte("RIFF")); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, 36+dataSize); err != nil {
		return err
	}
	if _, err := f.Write([]byte("WAVEfmt ")); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(16)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint16(1)); err != nil { // PCM
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint16(1)); err != nil { // mono
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(sampleRate)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(sampleRate*2)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint16(2)); err != nil { // block align
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint16(16)); err != nil { // bits per sample
		return err
	}
	if _, err := f.Write([]byte("data")); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, dataSize); err != nil {
		return err
	}
	for _, v := range data {
		if err := binary.Write(f, binary.LittleEndian, v); err != nil {
			return err
		}
	}
	return nil
}

func TestRegisterWAVPlaysSample(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.wav")
	if err := writeTestWAV(path); err != nil {
		t.Fatalf("writeTestWAV: %v", err)
	}
	if err := RegisterWAV("testwav", path); err != nil {
		t.Fatalf("RegisterWAV: %v", err)
	}
	instMu.RLock()
	inst, ok := instruments["testwav"]
	instMu.RUnlock()
	if !ok {
		t.Fatalf("instrument not registered")
	}
	m := &mixer{}
	m.Schedule(inst.NewVoice(120, sampleRate), 0)
	buf := make([]byte, sampleRate/100*2)
	m.Read(buf)
	nonZero := false
	for _, b := range buf {
		if b != 0 {
			nonZero = true
			break
		}
	}
	if !nonZero {
		t.Fatalf("expected non-zero audio output")
	}
}
