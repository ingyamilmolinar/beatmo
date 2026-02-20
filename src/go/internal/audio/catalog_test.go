//go:build test || js

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

func TestCatalogCategoriesSorted(t *testing.T) {
	withDefaultAudio(t)
	ResetCatalogForTest([]SoundMeta{
		{ID: "z-inst", Category: "Zebra"},
		{ID: "a-inst", Category: "Alpha"},
		{ID: "m-inst", Category: "Mid"},
		{ID: "a-inst2", Category: "Alpha"}, // duplicate category
	})
	cats := CatalogCategories()
	if len(cats) != 3 {
		t.Fatalf("expected 3 unique categories, got %d: %v", len(cats), cats)
	}
	if cats[0] != "Alpha" || cats[1] != "Mid" || cats[2] != "Zebra" {
		t.Fatalf("expected sorted [Alpha, Mid, Zebra], got %v", cats)
	}
}

func TestWavCategoryStub(t *testing.T) {
	cases := []struct{ in, want string }{
		{"snare", "Snares (WAV)"},
		{"kick", "Kick Drums (WAV)"},
		{"hihat", "Hi-Hats (WAV)"},
		{"hi-hat", "Hi-Hats (WAV)"},
		{"hi hat", "Hi-Hats (WAV)"},
		{"cymbals", "Cymbals (WAV)"},
		{"toms", "Toms (WAV)"},
		{"percussion", "Percussion (WAV)"},
		{"saved", "Saved (WAV)"},
		{"unknown", "Samples (WAV)"},
	}
	for _, tc := range cases {
		got := wavCategoryStub(tc.in)
		if got != tc.want {
			t.Errorf("wavCategoryStub(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSynthCategoryStub(t *testing.T) {
	cases := []struct{ in, want string }{
		{"snare", "Snares (Synth)"},
		{"kick", "Kick Drums (Synth)"},
		{"hihat", "Hi-Hats (Synth)"},
		{"tom", "Toms (Synth)"},
		{"clap", "Claps (Synth)"},
		{"cowbell", "Cowbells (Synth)"},
		{"ride", "Cymbals (Synth)"},
		{"crash", "Cymbals (Synth)"},
		{"shaker", "Percussion (Synth)"},
		{"bass", "Bass (Synth)"},
		{"rimshot", "Snares (Synth)"},
		{"sidestick", "Snares (Synth)"},
		{"unknown", "Synth (Other)"},
	}
	for _, tc := range cases {
		got := synthCategoryStub(tc.in)
		if got != tc.want {
			t.Errorf("synthCategoryStub(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSlugPathStub(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Snare/Hit 01", "snare-hit-01"},
		{"kick_heavy", "kick-heavy"},
		{"path/to/file", "path-to-file"},
		{"--leading--", "leading"},
		{"UPPER CASE", "upper-case"},
	}
	for _, tc := range cases {
		got := slugPathStub(tc.in)
		if got != tc.want {
			t.Errorf("slugPathStub(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestIsRegisteredUninitialized(t *testing.T) {
	withDefaultAudio(t)
	ResetCatalogForTest(nil)
	if IsRegistered("nonexistent") {
		t.Fatal("expected IsRegistered=false on fresh state")
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
