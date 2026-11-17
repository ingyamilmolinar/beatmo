//go:build !test && !js

package audio

import (
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// SongRenderInstrument / SongRenderNote are the offline-render request types for
// RenderSongOffline (used by internal/audio/songrender's production path). They
// live in package audio because faithful rendering needs the unexported offline
// mixer + voice helpers.
type SongRenderInstrument struct {
	ID instrument.ID
	// Doc is the instrument's document from the circuit (nil = its library
	// copy).
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

// noteDuration is a note's node Duration factor (DurSec is that factor times
// the instrument's voice length, as songrender and the UI harness build it).
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
		// The circuit's document replaces the copy (replace, not merge); an
		// instrument the circuit carries none for plays its library copy.
		if in.Doc != nil {
			_ = SetInstrumentDoc(in.ID, *in.Doc)
		} else if _, ok := EnsureInstrumentDoc(in.ID); ok {
			ResetInstrumentParams(in.ID)
		}
		// Instrument volume is already folded into every note's Gain by
		// songrender/arrangement.go (and the test-build stub relies on that).
		// Setting the channel volume to in.Volume as well would square it
		// (0.5 → 0.25) in the master mix — an over-attenuated render. Keep the
		// channel at unity so instrument volume is applied exactly once.
		SetChannelVolume(channel.Of(in.ID), 1.0)
		SetInsertEffects(in.ID, in.Effects)
	}
}

// renderOnePass schedules the given notes into a fresh offline mixer and renders
// totalSamples through the full master chain. keep filters which instruments are
// audible this pass (empty string → all). ResetDSPState clears filter/LFO/tail state.
func renderOnePass(notes []SongRenderNote, sampleRate, totalSamples int, keep instrument.ID) []float64 {
	ResetDSPState()
	m := newTestMixer()
	for _, n := range notes {
		if keep != "" && n.InstID != keep {
			continue
		}
		// The note exactly as a live trigger plays it (playedVoice: melodic at
		// pitch and gated by its Duration, any other voice resampled by
		// 2^(pitch/12) / Duration) — this render used to ignore both. Offline:
		// not the audioLoop dispatch goroutine.
		v := playedVoiceOrigin(n.InstID, n.Pitch, noteDuration(n), sampleRate, originPrewarm)
		if v == nil {
			continue
		}
		// scaledVoice satisfies Voice via a POINTER receiver — must pass &scaledVoice.
		m.Schedule(channel.Of(n.InstID), &scaledVoice{v: v, gain: n.Gain}, n.AtSample, -1, -1)
	}
	return renderMixer(m, totalSamples)
}

// RenderSongOffline renders the master (all notes) and per-instrument stems.
// It returns the master mix (all instruments) and a stem per instrument ID.
// Each pass runs through the real offline mixer + master chain.
func RenderSongOffline(insts []SongRenderInstrument, notes []SongRenderNote, sampleRate, totalSamples int) ([]float64, map[instrument.ID][]float64, error) {
	applySongInstruments(insts)
	master := renderOnePass(notes, sampleRate, totalSamples, "")
	stems := make(map[instrument.ID][]float64, len(insts))
	for _, in := range insts {
		stems[in.ID] = renderOnePass(notes, sampleRate, totalSamples, in.ID)
	}
	return master, stems, nil
}
