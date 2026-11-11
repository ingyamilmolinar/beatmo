package audio

import (
	"fmt"
	"io"
	"sort"
	"sync"
)

// AudioFormat identifies an output audio encoding format.
type AudioFormat string

const (
	FormatWAV16 AudioFormat = "wav16"  // 16-bit PCM WAV
	FormatWAV24 AudioFormat = "wav24"  // 24-bit PCM WAV
	FormatWAV32 AudioFormat = "wav32f" // 32-bit float WAV
	FormatFLAC  AudioFormat = "flac"   // FLAC lossless
	FormatOGG   AudioFormat = "ogg"    // OGG Vorbis lossy
)

// AudioEncoder writes PCM samples to an output stream in a specific format.
type AudioEncoder interface {
	// Format returns the encoder's format identifier.
	Format() AudioFormat

	// FileExtension returns the file extension including the dot (e.g., ".wav").
	FileExtension() string

	// Encode writes PCM float64 samples (range [-1, 1]) at the given sample rate to w.
	Encode(w io.Writer, samples []float64, sampleRate int) error
}

var (
	encoderRegistryMu sync.RWMutex
	encoderRegistry   = map[AudioFormat]func() AudioEncoder{}
)

// RegisterEncoder registers an encoder constructor for the given format.
func RegisterEncoder(f AudioFormat, ctor func() AudioEncoder) {
	encoderRegistryMu.Lock()
	encoderRegistry[f] = ctor
	encoderRegistryMu.Unlock()
}

// NewEncoder creates a new encoder for the given format.
func NewEncoder(f AudioFormat) (AudioEncoder, error) {
	encoderRegistryMu.RLock()
	ctor, ok := encoderRegistry[f]
	encoderRegistryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unsupported audio format: %s", f)
	}
	return ctor(), nil
}

// AvailableFormats returns all registered audio format identifiers, sorted.
func AvailableFormats() []AudioFormat {
	encoderRegistryMu.RLock()
	defer encoderRegistryMu.RUnlock()
	fmts := make([]AudioFormat, 0, len(encoderRegistry))
	for f := range encoderRegistry {
		fmts = append(fmts, f)
	}
	sort.Slice(fmts, func(i, j int) bool { return fmts[i] < fmts[j] })
	return fmts
}

// FormatFromExtension returns the AudioFormat for the given file extension.
// Returns empty string if not recognized.
func FormatFromExtension(ext string) AudioFormat {
	switch ext {
	case ".wav":
		return FormatWAV24 // default WAV = 24-bit
	case ".flac":
		return FormatFLAC
	case ".ogg":
		return FormatOGG
	default:
		return ""
	}
}
