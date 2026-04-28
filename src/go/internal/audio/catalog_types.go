package audio

// SoundMeta describes an available sound without forcing the audio data to be
// decoded. It supports lazy loading by storing only lightweight facts.
//
// Scope vs Source: Source encodes how the sample is rendered ("wav", "synth").
// Scope encodes who owns the instrument: "builtin" (synthesised in-binary) and
// "shipped" (bundled WAV on disk) are populated today. "user", "project", and
// "remote" are reserved for the future server-side instrument library,
// project-bundled samples, and downloaded packs respectively. Favorites and
// project pins reference instruments by ID across every Scope.
type SoundMeta struct {
	ID         string // stable instrument ID
	Name       string // display name
	Category   string // top-level folder (e.g., Kick, Snare)
	RelPath    string // path relative to catalog root using forward slashes
	Path       string // filesystem path
	Source     string // "wav", "synth"
	Scope      string // "builtin", "shipped"; reserved: "user", "project", "remote"
	Size       int64  // bytes on disk
	DurationMS int    // approximate duration in milliseconds
	SampleRate int    // Hz, 0 if unknown
	Channels   int    // channel count, 0 if unknown
}
