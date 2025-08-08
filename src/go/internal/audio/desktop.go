//go:build !js && !wasm && !test

package audio

import (
	"bytes"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
)

var (
	ctx   *oto.Context
	once  sync.Once
	start = time.Now()
)

func initContext() {
	const sampleRate = 44100
	var (
		ready chan struct{}
		err   error
	)
	ctx, ready, err = oto.NewContext(&oto.NewContextOptions{
		SampleRate:   sampleRate,
		ChannelCount: 1,
		Format:       oto.FormatSignedInt16LE,
	})
	if err != nil {
		// leave ctx nil; Play will no-op
		return
	}
	<-ready
}

// Play renders a simple snare via white noise and an exponential decay envelope.
func Play(id string, when ...float64) {
	if id != "snare" {
		return
	}
	once.Do(initContext)
	if ctx == nil {
		return
	}
	go func() {
		if len(when) > 0 {
			delay := time.Duration(when[0]-Now()) * time.Second
			if delay > 0 {
				time.Sleep(delay)
			}
		}
		const sampleRate = 44100
		const dur = 200 * time.Millisecond
		samples := int(float64(sampleRate) * dur.Seconds())
		buf := make([]byte, samples*2)
		for i := 0; i < samples; i++ {
			// exponential decay envelope
			env := math.Exp(-4 * float64(i) / float64(samples))
			v := int16((rand.Float64()*2 - 1) * env * 32767)
			buf[2*i] = byte(v)
			buf[2*i+1] = byte(v >> 8)
		}
		p := ctx.NewPlayer(bytes.NewReader(buf))
		p.Play()
	}()
}

// Now returns seconds since program start.
func Now() float64 { return time.Since(start).Seconds() }
