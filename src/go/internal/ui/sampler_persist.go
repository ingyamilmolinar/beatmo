package ui

import (
	"encoding/binary"
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/userprefs"
)

// Lives in ui, the one layer importing both, so userprefs stays audio-agnostic and audio stays
// userprefs-agnostic.
type samplePersistAdapter struct {
	store userprefs.SampleStore
}

// nil for a nil store, so callers can wire unconditionally.
func NewSamplePersistAdapter(store userprefs.SampleStore) *samplePersistAdapter {
	if store == nil {
		return nil
	}
	return &samplePersistAdapter{store: store}
}

// SaveSample implements audio.SampleSink.
func (a *samplePersistAdapter) SaveSample(id instrument.ID, pcm []float32, sr int) {
	if a == nil || a.store == nil {
		return
	}
	_ = a.store.SaveSample(id, userprefs.SampleBlob{SampleRate: sr, PCM: f32ToBytesLE(pcm)})
}

// DeleteSample implements audio.SampleSink.
func (a *samplePersistAdapter) DeleteSample(id instrument.ID) {
	if a == nil || a.store == nil {
		return
	}
	_ = a.store.DeleteSample(id)
}

// LoadSamples implements audio.SampleSource.
func (a *samplePersistAdapter) LoadSamples() map[instrument.ID]audio.SampleRecord {
	out := map[instrument.ID]audio.SampleRecord{}
	if a == nil || a.store == nil {
		return out
	}
	blobs, err := a.store.LoadSamples()
	if err != nil {
		return out
	}
	for id, b := range blobs {
		out[id] = audio.SampleRecord{PCM: bytesToF32LE(b.PCM), SampleRate: b.SampleRate}
	}
	return out
}

func f32ToBytesLE(pcm []float32) []byte {
	buf := make([]byte, len(pcm)*4)
	for i, v := range pcm {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf
}

func bytesToF32LE(b []byte) []float32 {
	n := len(b) / 4
	out := make([]float32, n)
	for i := 0; i < n; i++ {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return out
}
