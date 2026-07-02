//go:build test

package audio

import (
	"testing"
)

func TestDecodeWAVToPCM_24bit(t *testing.T) {
	pcm, sr, err := DecodeWAVToPCM("../../../../356181__mtg__violin-d5.wav")
	if err != nil {
		t.Fatalf("decode 24-bit: %v", err)
	}
	if sr != 48000 {
		t.Fatalf("sample rate = %d, want 48000", sr)
	}
	if len(pcm) < 100000 {
		t.Fatalf("len(pcm) = %d, want > 100000 (~2.6s @ 48k)", len(pcm))
	}
	var peak float32
	for _, s := range pcm {
		if s > peak {
			peak = s
		} else if -s > peak {
			peak = -s
		}
	}
	if peak < 0.05 || peak > 1.0 {
		t.Fatalf("peak = %.4f, want in (0.05, 1.0]", peak)
	}
}
