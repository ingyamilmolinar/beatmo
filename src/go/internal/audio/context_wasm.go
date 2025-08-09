//go:build js && wasm && !test

package audio

import "github.com/ebitengine/oto/v3"

func platformInitContext(sampleRate int) *oto.Context {
	ctx, ready, err := oto.NewContext(&oto.NewContextOptions{
		SampleRate:   sampleRate,
		ChannelCount: 1,
		Format:       oto.FormatSignedInt16LE,
	})
	if err != nil {
		return nil
	}
	go func() { <-ready }()
	_ = ctx.Resume()
	return ctx
}
