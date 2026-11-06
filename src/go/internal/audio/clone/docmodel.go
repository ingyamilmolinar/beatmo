package clone

import (
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// docmodel.go — the document's model read back as the modes and noise bands it
// SOUNDS at a node pitch (audio.ModalModes mirrors src/c/modal_bank.c line for
// line). Invert's identity test and the loss's prior term read it.

// modularBaseFreq is MODULAR_BASE_FREQ (src/c/modular.c): pitch 0 sounds A3.
const modularBaseFreq = 220.0

func stageOn(d instrument.Doc, stage string) bool {
	st, ok := d.Stages[stage]
	return ok && st.On
}

// docVoice: the voice frequency and the playback resample at node pitch
// (audio.ModalVoice over the document's flattened fields).
func docVoice(d instrument.Doc, pitch float64) (voiceHz, k float64) {
	return audio.ModalVoice(audio.FlatFieldMap(d), pitch, d.Render.Melodic)
}

// DocModes returns the modes d's MODAL (+ STRIKE) stages sound at node pitch
// when rendered at rate, in heard time (after the playback resample). Band is
// the table row. Rows the C skips (non-positive or above the limit, or past
// Keep) are absent. The mirror of the C lives in audio.ModalModes (the Synth
// tab's mode graph draws the same function).
func DocModes(d instrument.Doc, pitch float64, rate int) []Mode {
	if !stageOn(d, "modal") {
		return nil
	}
	mm := audio.ModalModes(audio.FlatFieldMap(d), pitch, d.Render.Melodic, rate)
	out := make([]Mode, len(mm))
	for i, m := range mm {
		out[i] = Mode{Freq: m.Freq, Alpha: m.Alpha, Amp: m.Amp, Phase: m.Phase, Band: m.Row}
	}
	return out
}

// DocNoise returns d's NOISEBAND rows in heard time at node pitch (centres
// and rates × the playback resample, attacks ÷ it), levels including the
// stage's offset.
func DocNoise(d instrument.Doc, pitch float64) []NoiseBand {
	st, ok := d.Stages["noiseband"]
	if !ok || !st.On || st.Table == nil {
		return nil
	}
	_, k := docVoice(d, pitch)
	col := tableCols(st.Table)
	var out []NoiseBand
	for _, row := range st.Table.Rows {
		out = append(out, NoiseBand{
			CenterHz: row[col["center"]] * k, Q: row[col["q"]],
			LevelDB:   row[col["level"]] + st.Values["noiseband_level"],
			AttackSec: row[col["attack"]] / k,
			FastRate:  row[col["fast_rate"]] * k, FastMix: row[col["fast_mix"]], TailRate: row[col["tail_rate"]] * k,
			Active: true,
		})
	}
	return out
}

func tableCols(t *instrument.Table) map[string]int {
	m := make(map[string]int, len(t.Cols))
	for i, c := range t.Cols {
		m[c] = i
	}
	return m
}
