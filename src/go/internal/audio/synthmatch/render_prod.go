//go:build !test

package synthmatch

import (
	"fmt"
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// RenderInstrument renders instID at the given pitch (semitones from A3=220Hz)
// for durSec (0 = instrument default) at sr, returning a mono wave.Wave (float64).
func RenderInstrument(instID instrument.ID, pitch float64, sr int, durSec float64) (wave.Wave, error) {
	return RenderWithParams(instID, pitch, sr, durSec, nil)
}

// RenderWithParams is RenderInstrument with extra param overrides set through
// the instrument's document (and "pitch") before rendering.
//
// Production build: renders the document's flattened block through the C DSP
// for bit-accurate audio. No normalization, no fallback — silence is a valid
// result.
func RenderWithParams(instID instrument.ID, pitch float64, sr int, durSec float64, overrides map[string]float64) (wave.Wave, error) {
	fields, _, samples, err := resolveRender(instID, pitch, sr, durSec, overrides)
	if err != nil {
		return wave.Wave{}, err
	}
	buf, err := audio.RenderInstrumentWithFields(instID, sr, samples, fields) // raw output; NO normalization
	if err != nil {
		return wave.Wave{}, fmt.Errorf("synthmatch: %w", err)
	}
	out := make([]float64, samples)
	for i, s := range buf {
		out[i] = float64(s)
	}
	return wave.Wave{Samples: out, SampleRate: sr, Label: string(instID)}, nil
}

// RenderShipped renders instID (with overrides set through its document) as
// a trigger PLAYS it — document FX, gate tail, normalisation, amplitude, and
// percussion pitched by resampling — via audio.RenderAsShipped, then fits it
// to durSec (0 = the instrument default): truncated, or zero-padded past the
// end of the voice (silence is what plays there). The comparison tools use
// this; RenderWithParams stays the RAW C render for the tools that need it
// (measure-loudness computes the amplitude, so it must not see it applied).
func RenderShipped(instID instrument.ID, pitch float64, sr int, durSec float64, overrides map[string]float64) (wave.Wave, error) {
	if durSec <= 0 {
		durSec = audio.ConfigForInstrument(instID).DurationSec
	}
	if int(float64(sr)*durSec) < 1 {
		return wave.Wave{}, fmt.Errorf("synthmatch: 0 samples for dur=%v sr=%d", durSec, sr)
	}
	return RenderHeld(instID, pitch, sr, durSec, 0, overrides)
}

// RenderHeld is RenderShipped with the voice held voiceSec long (its
// render.seconds; 0 = the shipped length) — a player holding the note longer
// or shorter, exactly as a document of that length plays. durSec <= 0
// returns the whole voice instead of fitting it to a length.
func RenderHeld(instID instrument.ID, pitch float64, sr int, durSec, voiceSec float64, overrides map[string]float64) (wave.Wave, error) {
	ov := make(map[string]float64, len(overrides))
	for k, v := range overrides {
		if k != "pitch" {
			ov[k] = v
		}
	}
	d, err := audio.InstrumentDocWithFields(instID, ov)
	if err != nil {
		return wave.Wave{}, fmt.Errorf("synthmatch: %w", err)
	}
	w, err := RenderHeldDoc(d, pitch, sr, durSec, voiceSec)
	w.Label = string(instID)
	return w, err
}

// RenderHeldDoc is RenderHeld for a document that is not registered anywhere
// (a tuned or cloned candidate); durSec <= 0 returns the whole voice.
func RenderHeldDoc(d instrument.Doc, pitch float64, sr int, durSec, voiceSec float64) (wave.Wave, error) {
	if voiceSec > 0 {
		d.Render.Seconds = voiceSec
	}
	v := audio.RenderAsShipped(d, sr, pitch)
	if durSec <= 0 {
		return wave.Wave{Samples: v, SampleRate: sr, Label: string(d.ID)}, nil
	}
	out := make([]float64, int(float64(sr)*durSec))
	copy(out, v)
	return wave.Wave{Samples: out, SampleRate: sr, Label: string(d.ID)}, nil
}

// HeldSeconds is how long instID's voice must be held, played at pitch, to
// last at least minSec: its shipped render.seconds when that is already long
// enough, longer otherwise (never shorter). Note length is the player's
// choice, so a comparison against a recording that holds its note for 2 s
// must not hold a 1 s voice against it.
func HeldSeconds(instID instrument.ID, pitch, minSec float64) float64 {
	d, err := audio.InstrumentDocWithFields(instID, nil)
	if err != nil {
		return 0
	}
	return HeldSecondsDoc(d, pitch, minSec)
}

// HeldSecondsDoc is HeldSeconds for a document that is not registered.
func HeldSecondsDoc(d instrument.Doc, pitch, minSec float64) float64 {
	rate := 1.0
	if !d.Render.Melodic {
		rate = math.Pow(2, pitch/12) // a resampled voice shortens by its rate
	}
	return math.Max(d.Render.Seconds, minSec*rate)
}

// RenderShippedDoc is RenderShipped for a document that is not registered
// anywhere (a WAV-clone candidate): the same played sound, durSec of it
// (0 = the document's own render.seconds), zero-padded past the voice.
func RenderShippedDoc(d instrument.Doc, pitch float64, sr int, durSec float64) (wave.Wave, error) {
	if durSec <= 0 {
		durSec = d.Render.Seconds
	}
	n := int(float64(sr) * durSec)
	if n < 1 {
		return wave.Wave{}, fmt.Errorf("synthmatch: 0 samples for dur=%v sr=%d", durSec, sr)
	}
	v := audio.RenderAsShipped(d, sr, pitch)
	out := make([]float64, n)
	copy(out, v)
	return wave.Wave{Samples: out, SampleRate: sr, Label: string(d.ID)}, nil
}
