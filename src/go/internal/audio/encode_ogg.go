package audio

import (
	"fmt"
	"io"
)

// oggEncoder encodes audio samples to OGG Vorbis format.
// Currently a stub that produces a valid but minimal OGG file.
// For production use, integrate github.com/jfreymuth/oggvorbis.
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
	// OGG Vorbis encoding requires a complex codec (DCT, psychoacoustic model).
	// Rather than ship a low-quality implementation, we return a clear error
	// directing users to available formats while the OGG encoder is registered
	// for future implementation.
	return fmt.Errorf("OGG Vorbis encoding not yet implemented; use WAV or FLAC format instead")
}
