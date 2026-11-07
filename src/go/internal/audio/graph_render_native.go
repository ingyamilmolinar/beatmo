//go:build !test && !js

package audio

import "github.com/ingyamilmolinar/beatmo/internal/instrument"

// Melodic documents render AT pitch; the rest are resampled after the Sampler edit, as playback does. Pure: writes
// no cache or global, so it may run on any goroutine.
func RenderDocForGraph(id instrument.ID, d instrument.Doc, pitch float64) ([]float32, int, error) {
	sr := SampleRate()
	melodic := d.Render.Melodic
	buf := renderDocVoice(d, sr, pitch, melodic)
	if e, ok := SampleEditFor(id); ok {
		buf = ApplySampleEditToBuffer(buf, sr, e)
	}
	if !melodic && pitch != 0 {
		v := &resampleVoice{src: &cVoice{buf: buf}, step: pow2(pitch / 12.0), buf: make([]float64, 0, len(buf)+2)}
		out := make([]float32, 0, len(buf))
		for {
			s, done := v.Sample()
			if done {
				break
			}
			out = append(out, float32(s))
		}
		buf = out
	}
	return buf, sr, nil
}

// GraphVoiceSec is the note's played length: render.seconds, in whole frames.
func GraphVoiceSec(d instrument.Doc) float64 {
	return float64(VoiceFrames(d.Render.Seconds, SampleRate())) / float64(SampleRate())
}
