//go:build !js && !wasm && !test

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
		BufferSize:   20 * time.Millisecond, // 20ms buffer reduces underruns/clicks on slower systems
	})
	if err != nil {
		return nil
	}
	<-ready
	return ctx
}
