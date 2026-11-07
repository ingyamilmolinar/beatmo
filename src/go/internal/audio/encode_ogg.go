package audio

import (
	"fmt"
	"io"
)

// A placeholder: encoding returns an error.
type oggEncoder struct{}

func init() {
	RegisterEncoder(FormatOGG, func() AudioEncoder { return &oggEncoder{} })
}

func (e *oggEncoder) Format() AudioFormat   { return FormatOGG }
func (e *oggEncoder) FileExtension() string { return ".ogg" }

func (e *oggEncoder) Encode(w io.Writer, samples []float64, sampleRate int) error {
	if len(samples) == 0 {
		return fmt.Errorf("no samples to encode")
	}
	// Vorbis needs a full codec; a clear error beats shipping a low-quality one.
	return fmt.Errorf("OGG Vorbis encoding not yet implemented; use WAV or FLAC format instead")
}
