//go:build !test && !js

package audio

// SongRenderInstrument / SongRenderNote are the offline-render request types for
// RenderSongOffline (used by internal/audio/songrender's production path). They
// live in package audio because faithful rendering needs the unexported offline
// mixer + send-effects + recipe-aware voice helpers.
type SongRenderInstrument struct {
	ID, Recipe  string
	SynthParams map[string]float64
	Volume, Pan float64
	ReverbSend  float64
	DelaySend   float64
	Effects     []EffectSlot
}

type SongRenderNote struct {
	InstID   string
	AtSample int
	Pitch    float64
	Gain     float64
	DurSec   float64
}

// applySongInstruments configures the global engine for one render pass.
func applySongInstruments(insts []SongRenderInstrument) {
	for _, in := range insts {
		if in.Recipe != "" {
			BindInstrumentToRecipe(in.ID, in.Recipe)
		}
		if len(in.SynthParams) > 0 {
			SetInstrumentParams(in.ID, RecipeParams(in.SynthParams))
		}
		SetChannelVolume(in.ID, in.Volume)
		SetChannelPan(in.ID, in.Pan)
		SetReverbSend(in.ID, in.ReverbSend)
		SetDelaySend(in.ID, in.DelaySend)
		SetInsertEffects(in.ID, in.Effects)
	}
}

// renderOnePass schedules the given notes into a fresh offline mixer and renders
// totalSamples through the full master chain. keep filters which instruments are
// audible this pass (empty string → all). resetSendEffectsState clears the reverb tail.
func renderOnePass(notes []SongRenderNote, bpm, sampleRate, totalSamples int, keep string) []float64 {
	resetSendEffectsState()
	m := newTestMixer()
	for _, n := range notes {
		if keep != "" && n.InstID != keep {
			continue
		}
		v := newRecipeAwareVoicePitched(n.InstID, bpm, sampleRate, n.Pitch)
		if v == nil {
			continue
		}
		// scaledVoice satisfies Voice via a POINTER receiver — must pass &scaledVoice.
		m.Schedule(n.InstID, &scaledVoice{v: v, gain: n.Gain}, n.AtSample)
	}
	return renderMixer(m, totalSamples)
}

// RenderSongOffline renders the master (all notes) and per-instrument stems.
// It returns the master mix (all instruments) and a stem per instrument ID.
// Each pass runs through the real offline mixer + master chain.
func RenderSongOffline(insts []SongRenderInstrument, notes []SongRenderNote, bpm, sampleRate, totalSamples int) ([]float64, map[string][]float64, error) {
	applySongInstruments(insts)
	master := renderOnePass(notes, bpm, sampleRate, totalSamples, "")
	stems := make(map[string][]float64, len(insts))
	for _, in := range insts {
		stems[in.ID] = renderOnePass(notes, bpm, sampleRate, totalSamples, in.ID)
	}
	return master, stems, nil
}
