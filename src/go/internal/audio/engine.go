//go:build !test

package audio

import (
	"sort"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
)

const sampleRate = 44100

var (
	ctx   *oto.Context
	once  sync.Once
	mix   *mixer
	start = time.Now()
	bpm   = 120

	instruments = map[string]Instrument{}
	instMu      sync.RWMutex
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
	instMu.Lock()
	instruments[id] = inst
	instMu.Unlock()
}

func init() {
	Register("snare", Snare{})
	Register("kick", Kick{})
	once.Do(initContext)
}

func initContext() {
	c := platformInitContext(sampleRate)
	if c == nil {
		return
	}
	ctx = c
	mix = newMixer(c)
}

// Play schedules an instrument by ID at an optional future time.
func Play(id string, when ...float64) {
	instMu.RLock()
	inst, ok := instruments[id]
	instMu.RUnlock()
	if !ok {
		return
	}
	once.Do(initContext)
	if ctx == nil {
		return
	}
	_ = ctx.Resume()
	delay := 0
	if len(when) > 0 {
		d := when[0] - Now()
		if d > 0 {
			delay = int(d * sampleRate)
		}
	}
	mix.Schedule(inst.NewVoice(bpm, sampleRate), delay)
}

// Now returns seconds since program start.
func Now() float64 { return time.Since(start).Seconds() }

// Reset closes the current audio context so queued sounds are dropped.
func Reset() {
	ctx = nil
	mix = nil
	once = sync.Once{}
}

// Resume attempts to resume the underlying audio context.
func Resume() {
	once.Do(initContext)
	if ctx != nil {
		_ = ctx.Resume()
	}
}

// SetBPM updates the global tempo used when constructing new voices.
func SetBPM(b int) { bpm = b }

// Instruments returns the list of registered instrument IDs.
func Instruments() []string {
	instMu.RLock()
	ids := make([]string, 0, len(instruments))
	for id := range instruments {
		ids = append(ids, id)
	}
	instMu.RUnlock()
	sort.Strings(ids)
	return ids
}

// mixer mixes multiple voices into a single PCM stream.
type mixer struct {
	mu     sync.Mutex
	voices []*voiceState
	pos    int
	player *oto.Player
}

type voiceState struct {
	start int
	v     Voice
}

func newMixer(c *oto.Context) *mixer {
	m := &mixer{}
	p := c.NewPlayer(m)
	p.SetBufferSize(sampleRate / 100 * 2)
	p.Play()
	m.player = p
	return m
}

// Schedule adds a voice to start after delaySamples have elapsed.
func (m *mixer) Schedule(v Voice, delaySamples int) {
	m.mu.Lock()
	m.voices = append(m.voices, &voiceState{start: m.pos + delaySamples, v: v})
	m.mu.Unlock()
}

// Read implements io.Reader for oto.Player.
func (m *mixer) Read(p []byte) (int, error) {
	samples := len(p) / 2
	for i := 0; i < samples; i++ {
		var sum float64
		m.mu.Lock()
		for idx := 0; idx < len(m.voices); idx++ {
			vs := m.voices[idx]
			if m.pos >= vs.start {
				val, done := vs.v.Sample()
				sum += val
				if done {
					m.voices = append(m.voices[:idx], m.voices[idx+1:]...)
					idx--
				}
			}
		}
		m.mu.Unlock()
		if sum > 1 {
			sum = 1
		} else if sum < -1 {
			sum = -1
		}
		v := int16(sum * 32767)
		p[2*i] = byte(v)
		p[2*i+1] = byte(v >> 8)
		m.pos++
	}
	return len(p), nil
}
