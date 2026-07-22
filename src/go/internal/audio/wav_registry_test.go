//go:build test

package audio

import (
	"encoding/json"
	"testing"
)

func TestSha256HexStable(t *testing.T) {
	if Sha256Hex([]byte("abc")) != Sha256Hex([]byte("abc")) {
		t.Fatal("hash not stable")
	}
	if Sha256Hex([]byte("abc")) == Sha256Hex([]byte("abd")) {
		t.Fatal("hash collision on different input")
	}
	if len(Sha256Hex([]byte("abc"))) != 64 {
		t.Fatalf("hex length = %d, want 64", len(Sha256Hex([]byte("abc"))))
	}
}

func TestHashSamplePCMDeterministicAndSensitive(t *testing.T) {
	a := HashSamplePCM([]float32{0.1, -0.2, 0.3}, 48000)
	if a != HashSamplePCM([]float32{0.1, -0.2, 0.3}, 48000) {
		t.Error("pcm hash not deterministic")
	}
	if a == HashSamplePCM([]float32{0.1, -0.2, 0.31}, 48000) {
		t.Error("pcm hash insensitive to samples")
	}
	if a == HashSamplePCM([]float32{0.1, -0.2, 0.3}, 44100) {
		t.Error("pcm hash insensitive to sample rate")
	}
}

func TestWavRegistryVersioning(t *testing.T) {
	r := NewWavRegistry()
	v1 := r.Register("kick.wav", "hashA")
	if v1.Version != 1 {
		t.Fatalf("first version = %d, want 1", v1.Version)
	}
	if again := r.Register("kick.wav", "hashA"); again.Version != 1 {
		t.Errorf("same content got new version %d", again.Version)
	}
	v2 := r.Register("kick.wav", "hashB")
	if v2.Version != 2 {
		t.Errorf("changed content version = %d, want 2", v2.Version)
	}
	if got, ok := r.Lookup("kick.wav", "hashA"); !ok || got.Version != 1 {
		t.Errorf("lookup by hash failed: %+v ok=%v", got, ok)
	}
	if _, ok := r.Lookup("kick.wav", "hashZ"); ok {
		t.Error("unknown hash resolved")
	}
	if _, ok := r.Lookup("nope.wav", "hashA"); ok {
		t.Error("unknown name resolved")
	}
}

func TestWavRegistryJSONRoundTrip(t *testing.T) {
	r := NewWavRegistry()
	r.Register("kick.wav", "hashA")
	r.Register("kick.wav", "hashB")
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	r2 := NewWavRegistry()
	if err := json.Unmarshal(raw, r2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got, ok := r2.Lookup("kick.wav", "hashB"); !ok || got.Version != 2 {
		t.Errorf("round-trip lost version: %+v ok=%v", got, ok)
	}
	// A new registration after reload continues the sequence.
	if v := r2.Register("kick.wav", "hashC"); v.Version != 3 {
		t.Errorf("post-reload version = %d, want 3", v.Version)
	}
}
