package audio

// A fixed time, never beats, so a note sounds the same at every tempo; the same arithmetic as audio.js, so both
// platforms render the same frame count.
func VoiceFrames(seconds float64, sampleRate int) int {
	n := int(float64(sampleRate) * seconds)
	if n < 1 {
		n = 1
	}
	return n
}
