//go:build !test

package audio

import "testing"

func TestMixerPlaysSequentialVoices(t *testing.T) {
	m := &mixer{}
	m.Schedule(Snare{}.NewVoice(120, sampleRate), 0)
	m.Schedule(Snare{}.NewVoice(120, sampleRate), sampleRate/4)
	buf := make([]byte, sampleRate)
	m.Read(buf)
	first := -1
	second := -1
	for i := 0; i < len(buf)/2; i++ {
		v := int16(buf[2*i]) | int16(buf[2*i+1])<<8
		if v != 0 {
			if first == -1 {
				first = i
			} else if i > sampleRate/4 && second == -1 {
				second = i
				break
			}
		}
	}
	if first == -1 || second == -1 {
		t.Fatalf("expected two non-zero segments, got first=%d second=%d", first, second)
	}
}
