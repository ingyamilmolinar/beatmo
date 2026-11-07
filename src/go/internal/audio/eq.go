//go:build !js && !test

package audio

import (
	"github.com/ingyamilmolinar/beatmo/internal/channel"
)

// The C program runner over BuildEQProgram: the same program and C the browser's worklet runs.
func NewEQProcessor(sampleRate int, bands ...EQBand) Processor {
	return newCEQ(BuildEQProgram(sampleRate, bands))
}

// NewPeakingEQ convenience helper for a single peaking band.
func NewPeakingEQ(sampleRate int, freq, q, gainDB float64) Processor {
	return NewEQProcessor(sampleRate, EQBand{Kind: EQPeaking, Freq: freq, Q: q, GainDB: gainDB})
}

// NewShelfEQ convenience helper for low/high shelf bands.
func NewShelfEQ(sampleRate int, low bool, freq, q, gainDB float64) Processor {
	kind := EQHighShelf
	if low {
		kind = EQLowShelf
	}
	return NewEQProcessor(sampleRate, EQBand{Kind: kind, Freq: freq, Q: q, GainDB: gainDB})
}

// Insert effects are preserved. A same-shape edit (every detent of a band drag) retunes the live EQ and keeps its
// state, as the worklet does; a new shape (a mute, a filter switch) builds a fresh EQ on both platforms.
func SetChannelEQ(id channel.ID, sampleRate int, bands ...EQBand) {
	channelEQWrites.Add(1)
	prog := BuildEQProgram(sampleRate, bands)
	rec := eqRecord{ID: id, SampleRate: sampleRate, Bands: append([]EQBand(nil), bands...)}
	if prev := lookupChannelEQ(id); prev.proc != nil {
		if live, ok := prev.proc.(*cEQ); ok && live.update(prog) {
			rec.proc = live
			recordChannelEQ(id, rec)
			return
		}
	}
	rec.proc = newCEQ(prog)
	recordChannelEQ(id, rec)
	RebuildChannelWithEQ(id, rec.proc)
}

// ClearChannelProcessors removes EQ from the channel, preserving insert effects.
func ClearChannelProcessors(id channel.ID) {
	recordChannelEQ(id, eqRecord{})
	RebuildChannelWithEQ(id, nil)
}
