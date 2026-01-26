//go:build !test && !js

package audio

import (
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
)


const (
	sampleRate          = 44100
	bufferSizeBytes10ms = sampleRate / 100 * 2 // 10ms of 16-bit mono audio
)

var (
	ctx   *oto.Context
	once  sync.Once
	mix   *mixer
	start = time.Now()
	bpm   = 120

	instruments = map[string]Instrument{}
	instOrder   []string
	instMu      sync.RWMutex

	stopHookMu sync.RWMutex
	stopHook   func(string)
)

// Voice generates PCM samples in the range [-1,1].
type Voice interface {
	// Sample returns the next sample and whether the voice has finished.
	Sample() (float64, bool)
}

// Instrument constructs a new Voice instance when triggered.
type Instrument interface {
	NewVoice(bpm, sampleRate int) Voice
}

// Register makes an instrument available for playback by ID.
func Register(id string, inst Instrument) {
	created := false
	instMu.Lock()
	if _, exists := instruments[id]; !exists {
		instOrder = append(instOrder, id)
		created = true
	}
	instruments[id] = inst
	instMu.Unlock()
	if created {
		bumpInstrumentsVersion()
	}
	InstrumentChannel(id)
}

func init() {
	ResetInstruments()
}

func initContext() {
	c := platformInitContext(sampleRate)
	if c == nil {
		return
	}
	ctx = c
	mix = newMixer(c)
}
