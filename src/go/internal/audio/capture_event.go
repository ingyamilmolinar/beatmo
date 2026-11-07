package audio

import (
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

type MIDIEvent struct {
	Time     float64       // seconds from recording start
	Type     string        // "note_on", "note_off"
	Channel  int           // MIDI channel (0-15)
	Note     int           // MIDI note number
	Velocity int           // 0-127
	InstrID  instrument.ID // the instrument that triggered this event
}

// Channel 10 (index 9) is the GM percussion channel.
var MIDIMapping = map[instrument.ID]int{
	roster.Kick:      36, // Bass Drum 1
	roster.KickDeep:  35, // Acoustic Bass Drum
	roster.Snare:     38, // Acoustic Snare
	roster.Hihat:     42, // Closed Hi-Hat
	roster.Clap:      39, // Hand Clap
	roster.Tom:       45, // Low Tom
	roster.Cowbell:   56, // Cowbell
	roster.Rimshot:   37, // Side Stick
	roster.Sidestick: 37, // Side Stick (alias)
	roster.Ride:      51, // Ride Cymbal 1
	roster.Crash:     49, // Crash Cymbal 1
	roster.Shaker:    70, // Maracas
	roster.FmBass:    36, // Map to Bass Drum (melodic → percussion fallback)
	roster.FmBell:    80, // Mute Triangle
	roster.FmLead:    81, // Open Triangle
	roster.FmEpiano:  88, // High Q (GM2 extension)
	roster.FmPluck:   39, // Hand Clap (fallback)
}

// Acoustic Snare (38) when unmapped.
func MIDINoteForInstrument(id instrument.ID) int {
	if note, ok := MIDIMapping[id]; ok {
		return note
	}
	return 38 // default: snare
}
