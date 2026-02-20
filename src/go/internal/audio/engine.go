//go:build !test && !js

package audio

import (
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
)

// sampleRate is the audio output sample rate.
// Default is 44100 Hz for maximum compatibility.
// Override with AUDIO_SAMPLE_RATE environment variable.
var sampleRate = func() int {
	if s := os.Getenv("AUDIO_SAMPLE_RATE"); s != "" {
		if rate, err := strconv.Atoi(s); err == nil && rate > 0 {
			log.Printf("[AUDIO] Using custom sample rate: %d Hz", rate)
			return rate
		}
	}
	return 44100 // Default to 44100 for maximum compatibility
}()

// bufferSizeBytes10ms is 10ms of 16-bit mono audio
var bufferSizeBytes10ms = sampleRate / 100 * 2

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

// BlockVoice is an optional interface for voices that support bulk sample
// rendering. When a voice implements BlockVoice, the mixer uses SampleBlock
// instead of calling Sample() in a tight loop, reducing per-sample function
// call overhead.
type BlockVoice interface {
	Voice
	// SampleBlock fills dst with up to len(dst) samples and returns the
	// number of samples written and whether the voice is done.
	SampleBlock(dst []float64) (int, bool)
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
	// Initialize send effects (delay + reverb).
	initSendEffects(sampleRate)
	// Initialize insert effect chains with the correct sample rate.
	InitInsertChains(sampleRate)
	// Install master compressor for automatic gain management.
	SetupMasterCompressor(sampleRate)
}

// Close stops the audio player, clears all voices, and releases the audio
// device. Safe to call when mix is nil (e.g. audio was never initialized).
func Close() {
	if mix == nil {
		return
	}
	mix.mu.Lock()
	mix.voices = nil
	mix.mu.Unlock()
	if mix.player != nil {
		mix.player.Pause()
		mix.player.Close()
	}
}
