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
	// NO oto Context.Resume on any per-trigger path: the ALSA loop holds the context lock across the blocking device
	// write, so a Resume here serialises dispatch behind it (worst on xrun recovery). Resume happens once from the
	// transport path.
	delay := 0
	if len(when) > 0 {
		d := when[0] - Now()
		if d > 0 {
			delay = int(d * float64(sampleRate))
		}
	}
	// An audition runs on the UI goroutine, so it is attributed to prewarm; a dispatch origin would falsely trip the
	// DispatchRenders==0 gate.
	v := newRecipeAwareVoiceOrigin(id, sampleRate, originPrewarm)
	if v == nil {
		return
	}
	mix.Schedule(channel.Of(id), v, delay, -1, -1)
}

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

// Pitch in semitones, dur a multiplier; how they apply depends on the instrument (playedVoice). Fire-and-forget:
// no parity identity.
func PlayParams(id instrument.ID, vol, pitch, dur float64, when ...float64) {
	playParamsIdentity(id, vol, pitch, dur, -1, -1, 0, when...)
}

// (-1, -1) for fire-and-forget sounds.
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
	// The audioLoop's hot dispatch path must never take the oto device lock.
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

// Shared with RenderAsPlayed, so measuring what a node SOUNDS like goes through playback's own construction.
func playedVoice(id instrument.ID, pitch, dur float64, sampleRate int) Voice {
	return playedVoiceOrigin(id, pitch, dur, sampleRate, originDispatch)
}

// An offline render is not the dispatch goroutine.
func playedVoiceOrigin(id instrument.ID, pitch, dur float64, sampleRate int, origin renderOrigin) Voice {
	// A melodic voice is re-rendered at the node pitch, so its formant stays at an absolute Hz instead of being slid by
	// resampling.
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
	// A melodic voice is ALREADY at pitch, so it plays at rate 1: its duration says how long it sounds, never what note
	// it is (below 1 it is gated with a short release). Drums and FM are pitched and sized by resampling: rate =
	// 2^(semitones/12) / dur.
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
	// Sized from the source length, so Sample() never regrows it.
	if cv := unwrapCVoice(v); cv != nil && len(cv.buf) > 0 {
		rv.buf = make([]float64, 0, len(cv.buf)+2)
	}
	return rv
}

// What a trigger puts into the mix before volume, through the playback path itself; a bare render at the node pitch
// does not reproduce it.
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

// No varargs allocation on the hot path; when <= 0 starts now; rowAbs is (row, abs), or (-1, -1) when omitted.
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

// The browser answers the same from its rendered buffers.
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

// nil clears it.
func SetStopHook(fn func(instrument.ID)) {
	stopHookMu.Lock()
	stopHook = fn
	stopHookMu.Unlock()
}
