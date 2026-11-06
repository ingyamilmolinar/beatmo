package audio

// VoiceFrames is the length of a voice whose document says render.seconds =
// seconds, at sampleRate: a fixed time, never a number of beats, so a note
// sounds the same at every tempo. It is the same arithmetic as audio.js
// (Math.max(1, Math.floor(sr * seconds))), so both platforms render the same
// number of frames. Guards: TestVoiceIsTheSameAtEveryTempo,
// voice_tempo_independence.browser.test.js.
func VoiceFrames(seconds float64, sampleRate int) int {
	n := int(float64(sampleRate) * seconds)
	if n < 1 {
		n = 1
	}
	return n
}
