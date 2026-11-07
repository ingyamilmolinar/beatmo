//go:build !test

package synthmatch

import (
	"fmt"
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// Pitch in semitones from A3 = 220 Hz; durSec 0 = the instrument default.
func RenderInstrument(instID instrument.ID, pitch float64, sr int, durSec float64) (wave.Wave, error) {
	return RenderWithParams(instID, pitch, sr, durSec, nil)
}

// The RAW C render: no normalization, no fallback; silence is a valid result.
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

// As a trigger PLAYS it, fitted to durSec (padded with the silence that plays there). RenderWithParams stays raw for
// tools that must not see the amplitude applied (measure-loudness computes it).
func RenderShipped(instID instrument.ID, pitch float64, sr int, durSec float64, overrides map[string]float64) (wave.Wave, error) {
	if durSec <= 0 {
		durSec = audio.ConfigForInstrument(instID).DurationSec
	}
	if int(float64(sr)*durSec) < 1 {
		return wave.Wave{}, fmt.Errorf("synthmatch: 0 samples for dur=%v sr=%d", durSec, sr)
	}
	return RenderHeld(instID, pitch, sr, durSec, 0, overrides)
}

// A player holding the note longer or shorter, exactly as a document of that length plays. durSec ≤ 0 returns the
// whole voice.
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

// For an unregistered document (a tuned or cloned candidate).
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

// Never shorter than shipped: note length is the player's choice, so a 2 s recorded note is not compared against a 1 s
// voice.
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

// For an unregistered document; durSec 0 = its own render.seconds.
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
