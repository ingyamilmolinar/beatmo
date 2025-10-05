//go:build !test && !js

package audio

import (
	"math"
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
	instMu.Lock()
	if _, exists := instruments[id]; !exists {
		instOrder = append(instOrder, id)
	}
	instruments[id] = inst
	instMu.Unlock()
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
	mix.Schedule(id, inst.NewVoice(bpm, sampleRate), delay)
}

// PlayVol schedules an instrument by ID at the given volume (0..1) and
// optional future time.
func PlayVol(id string, vol float64, when ...float64) {
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
	mix.Schedule(id, &scaledVoice{v: inst.NewVoice(bpm, sampleRate), gain: vol}, delay)
}

// PlayParams schedules an instrument with volume, pitch (in semitones), and
// duration multiplier. Pitch and duration are applied via naive resampling
// where playback rate r = 2^(pitch/12) / max(dur, eps). This couples pitch and
// duration (time-stretch not implemented), but allows practical control.
func PlayParams(id string, vol, pitch, dur float64, when ...float64) {
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
	v := inst.NewVoice(bpm, sampleRate)
	// Compute playback rate from semitones and requested duration multiplier.
	// r > 1 speeds up (higher pitch, shorter time). r < 1 slows down.
	rate := 1.0
	if dur <= 0 {
		dur = 1
	}
	// 2^(semitones/12)
	rate = pow2(pitch/12.0) / dur
	if rate <= 0 {
		rate = 1
	}
	rv := &resampleVoice{src: v, step: rate}
	mix.Schedule(id, &scaledVoice{v: rv, gain: vol}, delay)
}

// PlayParamsAt schedules with an explicit 'when' time in seconds. When 'when'
// is <= 0, playback starts immediately. This avoids varargs allocation in
// hot paths.
func PlayParamsAt(id string, vol, pitch, dur, when float64) {
	if when > 0 {
		PlayParams(id, vol, pitch, dur, when)
	} else {
		PlayParams(id, vol, pitch, dur)
	}
}

// SampleSeconds returns the base sample duration in seconds for an instrument ID.
// For built-in synths it approximates to a fraction of a beat; for WAV samples it
// returns the decoded buffer length.
func SampleSeconds(id string) float64 {
	instMu.RLock()
	inst, ok := instruments[id]
	b := bpm
	instMu.RUnlock()
	if !ok {
		return 0
	}
	switch id {
	case "snare":
		return (60.0 / float64(b)) * 0.25
	case "kick":
		return (60.0 / float64(b)) * 0.5
	case "hihat":
		return (60.0 / float64(b)) * 0.125
	case "tom":
		return (60.0 / float64(b)) * 0.5
	case "clap":
		return (60.0 / float64(b)) * 0.25
	}
	// Attempt to inspect sample length for WAVs.
	if s, ok := inst.(Sample); ok {
		return float64(len(s.data)) / float64(sampleRate)
	}
	return 0
}

// Stop immediately removes any active voices for the given instrument ID.
func Stop(id string) {
	instMu.RLock()
	_, ok := instruments[id]
	instMu.RUnlock()
	if !ok {
		return
	}
	once.Do(initContext)
	if mix != nil {
		mix.Stop(id)
	}
	stopHookMu.RLock()
	if stopHook != nil {
		stopHook(id)
	}
	stopHookMu.RUnlock()
}

// SetStopHook installs a callback invoked whenever Stop is called. Primarily
// used in tests; pass nil to clear the hook.
func SetStopHook(fn func(string)) {
	stopHookMu.Lock()
	stopHook = fn
	stopHookMu.Unlock()
}

// pow2 computes 2^x.
func pow2(x float64) float64 { return math.Pow(2, x) }

// resampleVoice wraps a source Voice and produces samples at a fractional
// rate using linear interpolation.
type resampleVoice struct {
	src  Voice
	step float64
	pos  float64
	buf  []float64
	done bool
}

func (r *resampleVoice) Sample() (float64, bool) {
	if r.step <= 0 {
		r.step = 1
	}
	// Ensure we have enough source samples to interpolate at current pos.
	need := int(r.pos) + 2
	for !r.done && len(r.buf) < need {
		s, d := r.src.Sample()
		r.buf = append(r.buf, s)
		if d {
			r.done = true
			break
		}
	}
	if len(r.buf) == 0 {
		return 0, true
	}
	i0 := int(r.pos)
	if i0 >= len(r.buf)-1 {
		if r.done {
			return 0, true
		}
		// Try to fetch one more sample.
		s, d := r.src.Sample()
		r.buf = append(r.buf, s)
		if d {
			r.done = true
		}
		if i0 >= len(r.buf)-1 && r.done {
			return 0, true
		}
	}
	var s0, s1 float64
	s0 = r.buf[i0]
	if i0+1 < len(r.buf) {
		s1 = r.buf[i0+1]
	} else {
		s1 = 0
	}
	frac := r.pos - float64(i0)
	out := s0 + (s1-s0)*frac
	r.pos += r.step
	return out, false
}

// ResetInstruments restores the built-in instrument set.
func ResetInstruments() {
	instMu.Lock()
	instruments = map[string]Instrument{
		"snare": Snare{},
		"kick":  Kick{},
		"hihat": HiHat{},
		"tom":   Tom{},
		"clap":  Clap{},
	}
	instOrder = []string{"snare", "kick", "hihat", "tom", "clap"}
	instMu.Unlock()
	resetInstrumentChannels(instOrder)
}

type scaledVoice struct {
	v    Voice
	gain float64
}

func (s *scaledVoice) Sample() (float64, bool) {
	f, done := s.v.Sample()
	return f * s.gain, done
}

// Now returns seconds since program start.
func Now() float64 { return time.Since(start).Seconds() }

// Reset closes the current audio context so queued sounds are dropped.
func Reset() {
	ctx = nil
	mix = nil
	once = sync.Once{}
	resetChannels()
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
	ids := append([]string(nil), instOrder...)
	instMu.RUnlock()
	return ids
}

// RenameInstrument updates the ID of an existing instrument.
func RenameInstrument(oldID, newID string) {
	instMu.Lock()
	if inst, ok := instruments[oldID]; ok {
		delete(instruments, oldID)
		instruments[newID] = inst
		for i, id := range instOrder {
			if id == oldID {
				instOrder[i] = newID
				break
			}
		}
	}
	instMu.Unlock()
	renameInstrumentChannel(oldID, newID)
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
	id    string
	v     Voice
	ch    *Channel
}

func newMixer(c *oto.Context) *mixer {
	m := &mixer{}
	p := c.NewPlayer(m)
	p.SetBufferSize(bufferSizeBytes10ms)
	p.Play()
	m.player = p
	return m
}

// Schedule adds a voice to start after delaySamples have elapsed.
func (m *mixer) Schedule(id string, v Voice, delaySamples int) {
	ch := channelForInstrument(id)
	m.mu.Lock()
	m.voices = append(m.voices, &voiceState{start: m.pos + delaySamples, id: id, v: v, ch: ch})
	m.mu.Unlock()
}

func (m *mixer) Stop(id string) {
	m.mu.Lock()
	for i := 0; i < len(m.voices); {
		if m.voices[i].id == id {
			m.voices = append(m.voices[:i], m.voices[i+1:]...)
			continue
		}
		i++
	}
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
				if done {
					m.voices = append(m.voices[:idx], m.voices[idx+1:]...)
					idx--
					continue
				}
				if vs.ch == nil {
					vs.ch = channelForInstrument(vs.id)
				}
				sum += vs.ch.ProcessSample(val)
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
