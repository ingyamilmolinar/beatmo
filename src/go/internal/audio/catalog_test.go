package audio

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureInstrumentLoadedRegistersOnDemand(t *testing.T) {
	withDefaultAudio(t)
	ResetCatalogForTest([]SoundMeta{
		{ID: "cat-snare-01", Name: "Snare 01", Category: "Snare", Path: "/tmp/snare.wav"},
	})
	if err := EnsureInstrumentLoaded("missing-id"); err != nil {
		if !strings.Contains(err.Error(), "not found") {
			t.Fatalf("unexpected error for missing id: %v", err)
		}
	}
	if IsRegistered("missing-id") {
		t.Fatalf("unexpected registration for missing id")
	}
	if IsRegistered("cat-snare-01") {
		t.Fatalf("instrument unexpectedly registered")
	}
	if err := EnsureInstrumentLoaded("cat-snare-01"); err != nil {
		t.Fatalf("ensure load failed: %v", err)
	}
	if !IsRegistered("cat-snare-01") {
		t.Fatalf("instrument not registered after ensure load")
	}
}

func TestInitCatalogFromDirRegistersWAVMetadata(t *testing.T) {
	withDefaultAudio(t)
	root := t.TempDir()
	catDir := filepath.Join(root, "snare")
	if err := os.MkdirAll(catDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(catDir, "hit.wav")
	if err := writeTinyWAV(path); err != nil {
		t.Fatalf("write wav: %v", err)
	}
	if err := InitCatalogFromDir(root); err != nil {
		t.Fatalf("init catalog failed: %v", err)
	}
	abs, _ := filepath.Abs(path)
	want := filepath.ToSlash(abs)
	found := false
	for _, m := range Catalog() {
		if filepath.ToSlash(m.Path) == want {
			found = true
			if len(m.Data) > 0 {
				t.Fatalf("wav data should not be eagerly loaded: %s", m.Path)
			}
			if m.Source != "wav" {
				t.Fatalf("expected wav source, got %q", m.Source)
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected wav to be registered: %s", want)
	}
}

func writeTinyWAV(path string) error {
	const sampleRate = 44100
	samples := sampleRate / 200 // 5ms
	data := make([]int16, samples)
	for i := range data {
		data[i] = int16(math.Sin(2*math.Pi*float64(i)/float64(samples)) * 12000)
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
	if err := binary.Write(f, binary.LittleEndian, uint16(1)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint16(1)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(sampleRate)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(sampleRate*2)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint16(2)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint16(16)); err != nil {
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
