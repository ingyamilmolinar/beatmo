//go:build !test

package audio

import (
	"testing"
	"time"
)

func TestVoiceStartsWithin50ms(t *testing.T) {
	m := &mixer{}
	m.Schedule(Snare{}.NewVoice(120, sampleRate), 0)
	buf := make([]byte, sampleRate/10*2) // 0.1s of 16-bit mono
	m.Read(buf)
	first := -1
	for i := 0; i < len(buf)/2; i++ {
		v := int16(buf[2*i]) | int16(buf[2*i+1])<<8
		if v != 0 {
			first = i
			break
		}
	}
	if first == -1 {
		t.Fatalf("no audio produced")
	}
	delay := time.Duration(first) * time.Second / sampleRate
	if delay > 50*time.Millisecond {
		t.Fatalf("start delay %v exceeds 50ms", delay)
	}
}
