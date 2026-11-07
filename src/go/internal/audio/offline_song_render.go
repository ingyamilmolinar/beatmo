//go:build !test && !js

package audio

import (
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// In package audio because faithful rendering needs the unexported offline mixer and voice helpers.
type SongRenderInstrument struct {
	ID instrument.ID
	// nil = its library copy.
	Doc     *instrument.Doc
	Volume  float64
	Effects []EffectSlot
}

type SongRenderNote struct {
	InstID   instrument.ID
	AtSample int
	Pitch    float64
	Gain     float64
	DurSec   float64
}

// DurSec is that factor times the instrument's voice length.
func noteDuration(n SongRenderNote) float64 {
	base := ConfigForInstrument(n.InstID).DurationSec
	if n.DurSec <= 0 || base <= 0 {
		return 1
	}
	return n.DurSec / base
}

// applySongInstruments configures the global engine for one render pass.
func applySongInstruments(insts []SongRenderInstrument) {
	for _, in := range insts {
		// Replace, not merge.
		if in.Doc != nil {
			_ = SetInstrumentDoc(in.ID, *in.Doc)
		} else if _, ok := EnsureInstrumentDoc(in.ID); ok {
			ResetInstrumentParams(in.ID)
		}
		// Instrument volume is already in each note's Gain; setting the channel too would square it.
		SetChannelVolume(channel.Of(in.ID), 1.0)
		SetInsertEffects(in.ID, in.Effects)
	}
}

// keep "" renders every instrument.
func renderOnePass(notes []SongRenderNote, sampleRate, totalSamples int, keep instrument.ID) []float64 {
	ResetDSPState()
	m := newTestMixer()
	for _, n := range notes {
		if keep != "" && n.InstID != keep {
			continue
		}
		// Exactly as a live trigger plays it: melodic at pitch and gated by Duration, anything else resampled.
		v := playedVoiceOrigin(n.InstID, n.Pitch, noteDuration(n), sampleRate, originPrewarm)
		if v == nil {
			continue
		}
		// scaledVoice satisfies Voice via a POINTER receiver — must pass &scaledVoice.
		m.Schedule(channel.Of(n.InstID), &scaledVoice{v: v, gain: n.Gain}, n.AtSample, -1, -1)
	}
	return renderMixer(m, totalSamples)
}

// The master mix plus a stem per instrument, each through the real offline mixer.
func RenderSongOffline(insts []SongRenderInstrument, notes []SongRenderNote, sampleRate, totalSamples int) ([]float64, map[instrument.ID][]float64, error) {
	applySongInstruments(insts)
	master := renderOnePass(notes, sampleRate, totalSamples, "")
	stems := make(map[instrument.ID][]float64, len(insts))
	for _, in := range insts {
		stems[in.ID] = renderOnePass(notes, sampleRate, totalSamples, in.ID)
	}
	return master, stems, nil
}
