//go:build !js

package userprefs

import (
	"bytes"
	"testing"
)

func TestSampleStoreFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewBackingStore(Options{Path: dir})
	ss, ok := store.(SampleStore)
	if !ok {
		t.Fatal("backing store does not implement SampleStore")
	}

	id := "user.sample.abc123"
	blob := SampleBlob{SampleRate: 44100, PCM: []byte{1, 2, 3, 4, 5, 6, 7, 8}}
	if err := ss.SaveSample(id, blob); err != nil {
		t.Fatalf("SaveSample: %v", err)
	}

	// A fresh store instance must read the sample back from disk.
	store2 := NewBackingStore(Options{Path: dir}).(SampleStore)
	loaded, err := store2.LoadSamples()
	if err != nil {
		t.Fatalf("LoadSamples: %v", err)
	}
	got, ok := loaded[id]
	if !ok {
		t.Fatalf("sample %q not loaded after reopen", id)
	}
	if got.SampleRate != 44100 || !bytes.Equal(got.PCM, blob.PCM) {
		t.Errorf("round-trip mismatch: got %+v want %+v", got, blob)
	}

	// Delete removes it across reopen.
	if err := store2.DeleteSample(id); err != nil {
		t.Fatalf("DeleteSample: %v", err)
	}
	store3 := NewBackingStore(Options{Path: dir}).(SampleStore)
	loaded3, _ := store3.LoadSamples()
	if _, ok := loaded3[id]; ok {
		t.Error("sample still present after DeleteSample + reopen")
	}
}

func TestSampleStoreEmptyDirIsNotError(t *testing.T) {
	dir := t.TempDir()
	ss := NewBackingStore(Options{Path: dir}).(SampleStore)
	loaded, err := ss.LoadSamples()
	if err != nil {
		t.Fatalf("LoadSamples on empty dir: %v", err)
	}
	if len(loaded) != 0 {
		t.Errorf("empty store returned %d samples", len(loaded))
	}
}
