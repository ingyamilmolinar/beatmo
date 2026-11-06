package audio

import (
	"github.com/ingyamilmolinar/beatmo/internal/enum"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// SoundSource is how a catalog sound is made.
type SoundSource int

const (
	SourceSynth SoundSource = iota // rendered by the synth from its document
	SourceWAV                      // a WAV file on disk
)

var soundSourceNames = enum.Names[SoundSource]{Kind: "sound source", Names: []string{"synth", "wav"}}

func (s SoundSource) String() string                { return enum.String(soundSourceNames, s) }
func (s SoundSource) MarshalText() ([]byte, error)  { return enum.Marshal(soundSourceNames, s) }
func (s *SoundSource) UnmarshalText(b []byte) error { return enum.Unmarshal(soundSourceNames, b, s) }

// SoundMeta describes an available sound without forcing the audio data to be
// decoded. It supports lazy loading by storing only lightweight facts.
type SoundMeta struct {
	ID         instrument.ID // stable instrument ID
	Name       string        // display name
	Group      string        // catalog display group ("Kick (Synth)", "Snares (WAV)"); the menu category is CategoryOf
	RelPath    string        // path relative to catalog root using forward slashes
	Path       string        // filesystem path
	Source     SoundSource   // how the sound is made
	Size       int64         // bytes on disk
	DurationMS int           // approximate duration in milliseconds
	SampleRate int           // Hz, 0 if unknown
	Channels   int           // channel count, 0 if unknown
}
