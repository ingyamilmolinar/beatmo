//go:build !test && !js

package audio

import (
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/env"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// Play schedules an instrument by ID at an optional future time.
func Play(id instrument.ID, when ...float64) {
	instMu.RLock()
	_, ok := instruments[id]
	instMu.RUnlock()
	if !ok {
		return
	}
	RecordVoiceTrigger(id)
	once.Do(initContext)
	if ctx == nil {
		return
	}
	// NO oto Context.Resume call here (or in any per-trigger path): the ALSA loop holds
	// the context lock across the BLOCKING snd_pcm_writei/snd_pcm_recover
	// device write (driver_unix.go readAndWrite), and Resume takes that same
	// lock — a per-trigger Resume serializes the audioLoop dispatch behind the
	// device writer (~one period typically, 30-100ms on xrun recovery), which
	// is what tripped the overdue/absmax timing-parity panic. The context is
	// never suspended during playback on desktop; resume-on-start is handled
	// by audio.Resume() (engine_state.go) from the transport path.
	delay := 0
	if len(when) > 0 {
		d := when[0] - Now()
		if d > 0 {
			delay = int(d * float64(sampleRate))
		}
	}
	// Synth/Sampler audition — the UI goroutine, not the audioLoop dispatch
	// goroutine. Must attribute originPrewarm, not the bare wrapper's default
	// originDispatch, or a knob-release audition would falsely trip the
	// DispatchRenders==0 gate.
	v := newRecipeAwareVoiceOrigin(id, sampleRate, originPrewarm)
	if v == nil {
		return
	}
	mix.Schedule(channel.Of(id), v, delay, -1, -1)
}

// PlayVol schedules an instrument by ID at the given volume (0..1) and
// optional future time.
func PlayVol(id instrument.ID, vol float64, when ...float64) {
	instMu.RLock()
	_, ok := instruments[id]
	instMu.RUnlock()
	if !ok {
		return
	}
	RecordVoiceTrigger(id)
	once.Do(initContext)
	if ctx == nil {
		return
	}
	// No per-trigger oto Resume — see the comment in Play above.
	delay := 0
	if len(when) > 0 {
		d := when[0] - Now()
		if d > 0 {
			delay = int(d * float64(sampleRate))
		}
	}
	// Synth/Sampler audition — UI goroutine; see the comment in Play above.
	inner := newRecipeAwareVoiceOrigin(id, sampleRate, originPrewarm)
	if inner == nil {
		return
	}
	mix.Schedule(channel.Of(id), &scaledVoice{v: inner, gain: vol}, delay, -1, -1)
}

// PlayParams schedules an instrument with volume, pitch (in semitones), and
// duration multiplier. Pitch and duration are applied via naive resampling
// where playback rate r = 2^(pitch/12) / max(dur, eps). This couples pitch and
// duration (time-stretch not implemented), but allows practical control.
//
// PlayParams delegates to playParamsIdentity with row=-1, abs=-1 (no parity
// identity) so every existing caller keeps working unchanged. Callers that
// know the sequencer identity of the sound should go through PlayParamsAt.
func PlayParams(id instrument.ID, vol, pitch, dur float64, when ...float64) {
	playParamsIdentity(id, vol, pitch, dur, -1, -1, 0, when...)
}

// playParamsIdentity is the implementation shared by PlayParams and
// PlayParamsAt; row/abs carry the sequencer's parity identity (row, abs) for
// this emission, or (-1, -1) for fire-and-forget / non-sequencer sounds.
func playParamsIdentity(id instrument.ID, vol, pitch, dur float64, row, abs int, vid uint64, when ...float64) {
	instMu.RLock()
	_, ok := instruments[id]
	instMu.RUnlock()
	if !ok {
		return
	}
	if vid == 0 { // lane voices: the UI lane stamps the trigger when the beat comes due
		RecordVoiceTrigger(id)
	}
	once.Do(initContext)
	if ctx == nil {
		return
	}
	// No per-trigger oto Resume — see the comment in Play above. This is the
	// sequencer's hot dispatch path (audioLoop goroutine); it must never take
	// the oto device lock.
	delay := 0
	if len(when) > 0 {
		d := when[0] - Now()
		if d > 0 {
			delay = int(d * float64(sampleRate))
		}
	}
	if env.PrewarmDebug.Get() && row >= 0 {
		log.Debugf("[pw] disp-call id=%s pitch=%.2f row=%d abs=%d pitchAware=%v", id, pitch, row, abs, IsMelodicInstrument(id))
	}
	rv := playedVoice(id, pitch, dur, sampleRate)
	if rv == nil {
		return
	}
	mix.ScheduleLane(channel.Of(id), &scaledVoice{v: rv, gain: vol}, delay, row, abs, vid)
}

// playedVoice builds the voice a sequencer trigger plays: the instrument's
// render at the node's pitch and duration, exactly as the mixer receives it.
// Shared by playParamsIdentity and RenderAsPlayed, so a measurement of what a
// node SOUNDS like goes through the same construction as playback.
func playedVoice(id instrument.ID, pitch, dur float64, sampleRate int) Voice {
	return playedVoiceOrigin(id, pitch, dur, sampleRate, originDispatch)
}

// playedVoiceOrigin is playedVoice with explicit render attribution (an
// offline render is not the audioLoop dispatch goroutine).
func playedVoiceOrigin(id instrument.ID, pitch, dur float64, sampleRate int, origin renderOrigin) Voice {
	// For pitch-aware melodic recipes: re-render at the node pitch so the
	// filter formant stays at an absolute Hz instead of being slid by
	// resampling. The playback rate then carries only the duration factor.
	// For all other instruments (drums, FM, etc.) the old path is unchanged.
	pitchAware := IsMelodicInstrument(id)
	var v Voice
	if pitchAware {
		v = newRecipeAwareVoicePitchedOrigin(id, sampleRate, pitch, origin)
	} else {
		v = newRecipeAwareVoiceOrigin(id, sampleRate, origin)
	}
	if v == nil {
		return nil
	}
	if dur <= 0 {
		dur = 1
	}
	// A pitch-aware voice is ALREADY at pitch, so it plays at rate 1: its
	// duration says how long it sounds, never what note it is. Below 1 the
	// note is gated short with a brief release; at or above 1 it plays its
	// full render (a render cannot be stretched without re-rendering it).
	//
	// This used to be rate = 1/dur — the duration applied as a playback
	// SPEED — so a node of duration 0.5 sounded an octave high and 0.25 two
	// octaves high. Every template writes note lengths there, so every
	// eighth and sixteenth of every melodic line was transposed (Ave Maria's
	// vocal line leapt by octaves). Guard: TestNodeDurationNeverChangesPitch.
	//
	// A non-pitch-aware voice (drums, FM) is still pitched AND sized by naive
	// resampling: rate = 2^(semitones/12) / dur, by design.
	var rate float64
	if pitchAware {
		rate = 1
		if cv, ok := v.(*cVoice); ok && dur < 1 {
			if n := int(float64(len(cv.buf)) * dur); n > 0 {
				v = newGateVoice(v, n, sampleRate)
			}
		}
	} else {
		// 2^(semitones/12)
		rate = pow2(pitch/12.0) / dur
	}
	if rate <= 0 {
		rate = 1
	}
	rv := &resampleVoice{src: v, step: rate}
	// Pre-allocate buffer capacity from known source length to avoid
	// repeated slice growth during Sample() calls.
	if cv := unwrapCVoice(v); cv != nil && len(cv.buf) > 0 {
		rv.buf = make([]float64, 0, len(cv.buf)+2)
	}
	return rv
}

// RenderAsPlayed returns the samples a sequencer trigger of id at (pitch, dur)
// puts into the mix — the same voice playParamsIdentity schedules, before
// volume — at the engine's sample rate, up to maxSeconds. It exists so tests
// can measure what a node SOUNDS like through the playback path itself, which
// a render of the bare recipe at the node pitch does not reproduce.
func RenderAsPlayed(id instrument.ID, pitch, dur, maxSeconds float64) ([]float64, int) {
	sr := SampleRate()
	v := playedVoice(id, pitch, dur, sr)
	if v == nil {
		return nil, sr
	}
	n := int(maxSeconds * float64(sr))
	out := make([]float64, 0, n)
	for len(out) < n {
		s, done := v.Sample()
		if done {
			break
		}
		out = append(out, s)
	}
	return out, sr
}

// PlayParamsAt schedules with an explicit 'when' time in seconds. When 'when'
// is <= 0, playback starts immediately. This avoids varargs allocation in
// hot paths. rowAbs optionally carries the sequencer's parity identity as
// (row, abs); omit it (or pass a slice of any other length) for
// fire-and-forget sounds, which are tagged (-1, -1).
func PlayParamsAt(id instrument.ID, vol, pitch, dur, when float64, rowAbs ...int) {
	row, abs := -1, -1
	if len(rowAbs) == 2 {
		row, abs = rowAbs[0], rowAbs[1]
	}
	if when > 0 {
		playParamsIdentity(id, vol, pitch, dur, row, abs, 0, when)
	} else {
		playParamsIdentity(id, vol, pitch, dur, row, abs, 0)
	}
}

// PlayLaneAt schedules an audio-lane voice (vid > 0) at audio time when.
func PlayLaneAt(id instrument.ID, vol, pitch, dur, when float64, row, abs int, vid uint64) {
	playParamsIdentity(id, vol, pitch, dur, row, abs, vid, when)
}

// SampleSeconds returns the base duration in seconds of an instrument's
// voice: a synth document's render.seconds, a WAV's decoded length, else 0.
// The browser answers the same from its rendered buffers (sampleDurationSec).
func SampleSeconds(id instrument.ID) float64 {
	instMu.RLock()
	inst, ok := instruments[id]
	instMu.RUnlock()
	if !ok {
		return 0
	}
	if s, ok := inst.(Sample); ok && !hasDocVoice(id) {
		return float64(len(s.data)) / float64(sampleRate)
	}
	if d, ok := InstrumentDoc(id); ok {
		return float64(VoiceFrames(d.Render.Seconds, sampleRate)) / float64(sampleRate)
	}
	return 0
}

// Stop immediately removes any active voices for the given instrument ID.
func Stop(id instrument.ID) {
	instMu.RLock()
	_, ok := instruments[id]
	instMu.RUnlock()
	if !ok {
		return
	}
	once.Do(initContext)
	if mix != nil {
		mix.Stop(channel.Of(id))
	}
	stopHookMu.RLock()
	if stopHook != nil {
		stopHook(id)
	}
	stopHookMu.RUnlock()
}

// SetStopHook installs a callback invoked whenever Stop is called. Primarily
// used in tests; pass nil to clear the hook.
func SetStopHook(fn func(instrument.ID)) {
	stopHookMu.Lock()
	stopHook = fn
	stopHookMu.Unlock()
}
