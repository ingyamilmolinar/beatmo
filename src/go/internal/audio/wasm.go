//go:build js && wasm && !test

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
	bpm   = 120
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
	// Drain ready asynchronously so Play doesn't block waiting for user
	// interaction to resume the Web Audio context.
	go func() { <-ready }()
	// Attempt to resume immediately; browsers may require a user gesture.
	_ = ctx.Resume()
}

// Play renders a simple snare via white noise and an exponential decay envelope.
func Play(id string, when ...float64) {
	if id != "snare" {
		return
	}
	once.Do(initContext)
	c := ctx
	if c == nil {
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
		spb := 60 / float64(bpm)
		dur := time.Duration(spb * 0.5 * float64(time.Second))
		samples := int(float64(sampleRate) * dur.Seconds())
		buf := make([]byte, samples*2)
		for i := 0; i < samples; i++ {
			env := math.Exp(-4 * float64(i) / float64(samples))
			v := int16((rand.Float64()*2 - 1) * env * 32767)
			buf[2*i] = byte(v)
			buf[2*i+1] = byte(v >> 8)
		}
		p := c.NewPlayer(bytes.NewReader(buf))
		p.Play()
	}()
}

// Now returns seconds since program start.
func Now() float64 { return time.Since(start).Seconds() }

// Reset closes the current audio context so queued sounds are dropped.
func Reset() {
	ctx = nil
	once = sync.Once{}
}

func SetBPM(b int) { bpm = b }
