//go:build js && wasm && !test

package audio

import (
	"time"

	"github.com/ebitengine/oto/v3"
)

func platformInitContext(sampleRate int) *oto.Context {
	ctx, ready, err := oto.NewContext(&oto.NewContextOptions{
		SampleRate:   sampleRate,
		ChannelCount: 1,
		Format:       oto.FormatSignedInt16LE,
		BufferSize:   10 * time.Millisecond,
	})
	if err != nil {
		return nil
	}
	go func() { <-ready }()
	return ctx
}
