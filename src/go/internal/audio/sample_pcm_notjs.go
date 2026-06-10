//go:build !test && !js

package audio

// factoryInstruments is the as-shipped instrument set, snapshotted by
// ResetInstruments. UnregisterSamplePCM restores a single entry from it so a
// factory Reset un-does the Sample that the Sampler's Save registered over a
// built-in.
var factoryInstruments map[string]Instrument

// RegisterSamplePCM registers an in-memory PCM buffer as a playable instrument
// (overriding any existing instrument with the same id). pcm must already be at
// the engine sample rate (SampleRate()); callers that load foreign-rate audio
// (WAV import) resample before calling. The sr argument is recorded for the
// browser AudioBuffer path and is unused natively because cVoice plays at the
// engine rate.
func RegisterSamplePCM(id string, pcm []float32, sr int) {
	Register(id, Sample{data: pcm})
}

// UnregisterSamplePCM removes the Sample registered for id and restores the
// instrument's as-shipped built-in render from the factory snapshot, so a
// factory Reset makes the synth audible again instead of the leftover chop.
// For a non-built-in id (a pure user sample) the entry is simply dropped.
func UnregisterSamplePCM(id string) {
	instMu.Lock()
	if inst, ok := factoryInstruments[id]; ok {
		instruments[id] = inst
	} else {
		delete(instruments, id)
	}
	instMu.Unlock()
	globalVoiceCache.Clear()
	bumpInstrumentsVersion()
}

// DecodeWAVToPCM decodes a WAV/audio file to mono PCM at the engine sample
// rate, resampling when the file's rate differs (closing the historical
// resample TODO so any-rate WAVs play at the correct speed). Used by the
// Sampler tab's "Load WAV" on desktop.
func DecodeWAVToPCM(path string) ([]float32, int, error) {
	buf, sr, err := loadAudio(path)
	if err != nil {
		return nil, 0, err
	}
	if sr != sampleRate {
		buf = ResampleToRate(buf, sr, sampleRate)
		sr = sampleRate
	}
	return buf, sr, nil
}
