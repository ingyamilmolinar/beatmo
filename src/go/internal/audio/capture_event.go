package audio

// MIDIEvent represents a MIDI event captured during recording.
// These events are collected alongside audio data to enable future MIDI export.
type MIDIEvent struct {
	Time       float64 // seconds from recording start
	Type       string  // "note_on", "note_off"
	Channel    int     // MIDI channel (0-15)
	Note       int     // MIDI note number
	Velocity   int     // 0-127
	InstrID    string  // Beatmo instrument ID that triggered this event
}

// MIDIMapping maps Beatmo instrument IDs to General MIDI percussion note numbers.
// Channel 10 (index 9) is the standard GM percussion channel.
var MIDIMapping = map[string]int{
	"kick":       36, // Bass Drum 1
	"kick-deep":  35, // Acoustic Bass Drum
	"snare":      38, // Acoustic Snare
	"hihat":      42, // Closed Hi-Hat
	"clap":       39, // Hand Clap
	"tom":        45, // Low Tom
	"cowbell":    56, // Cowbell
	"rimshot":    37, // Side Stick
	"sidestick":  37, // Side Stick (alias)
	"ride":       51, // Ride Cymbal 1
	"crash":      49, // Crash Cymbal 1
	"shaker":     70, // Maracas
	"fm-bass":    36, // Map to Bass Drum (melodic → percussion fallback)
	"fm-bell":    80, // Mute Triangle
	"fm-lead":    81, // Open Triangle
	"fm-epiano":  88, // High Q (GM2 extension)
	"fm-pluck":   39, // Hand Clap (fallback)
}

// MIDINoteForInstrument returns the GM percussion note number for a Beatmo instrument.
// Returns 38 (Acoustic Snare) as default if the instrument is not mapped.
func MIDINoteForInstrument(id string) int {
	if note, ok := MIDIMapping[id]; ok {
		return note
	}
	return 38 // default: snare
}
