package audio

// SoundMeta describes an available sound without forcing the audio data to be
// decoded. It supports lazy loading by storing only lightweight facts.
type SoundMeta struct {
	ID         string // stable instrument ID
	Name       string // display name
	Category   string // top-level folder (e.g., Kick, Snare)
	RelPath    string // path relative to catalog root using forward slashes
	Path       string // filesystem path when not embedded
	Source     string // "wav", "synth", "embedded"
	Size       int64  // bytes on disk (or embedded payload)
	DurationMS int    // approximate duration in milliseconds
	SampleRate int    // Hz, 0 if unknown
	Channels   int    // channel count, 0 if unknown
	Embedded   bool   // true when payload lives in the binary
	Data       []byte // embedded payload (not decoded); nil for disk assets
}
