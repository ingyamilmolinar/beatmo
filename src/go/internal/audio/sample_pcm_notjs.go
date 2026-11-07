//go:build !test && !js

package audio

import "github.com/ingyamilmolinar/beatmo/internal/instrument"

// Snapshotted by ResetInstruments so UnregisterSamplePCM can undo a Sample saved over a built-in.
var factoryInstruments map[instrument.ID]Instrument

// Overrides any instrument with the same id. pcm must already be at the engine rate; sr is used only by the browser.
func RegisterSamplePCM(id instrument.ID, pcm []float32, sr int) {
	bumpVoiceRev(id) // new PCM: the lane must replace this instrument's not-started notes
	Register(id, Sample{data: pcmAtEngineRate(pcm, sr, sampleRate)})
}

// Restores the as-shipped render so the synth is heard again, not the leftover chop; a pure user sample is dropped.
func UnregisterSamplePCM(id instrument.ID) {
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

// The Sampler's "Load WAV" on desktop.
func DecodeWAVToPCM(path string) ([]float32, int, error) { return DecodeWAVAtRate(path, sampleRate) }

// The reference tools read at the corpus rate (wavref.Rate) whatever the engine runs at, or every recording would
// come back resampled to 48 kHz.
func DecodeWAVAtRate(path string, rate int) ([]float32, int, error) {
	buf, sr, err := loadAudio(path)
	if err != nil {
		return nil, 0, err
	}
	if sr != rate {
		buf = ResampleToRate(buf, sr, rate)
		sr = rate
	}
	return buf, sr, nil
}
