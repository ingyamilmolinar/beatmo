//go:build !test && !js

package audio

// Play schedules an instrument by ID at an optional future time.
func Play(id string, when ...float64) {
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
	_ = ctx.Resume()
	delay := 0
	if len(when) > 0 {
		d := when[0] - Now()
		if d > 0 {
			delay = int(d * float64(sampleRate))
		}
	}
	v := newRecipeAwareVoice(id, bpm, sampleRate)
	if v == nil {
		return
	}
	mix.Schedule(id, v, delay)
}

// PlayVol schedules an instrument by ID at the given volume (0..1) and
// optional future time.
func PlayVol(id string, vol float64, when ...float64) {
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
	_ = ctx.Resume()
	delay := 0
	if len(when) > 0 {
		d := when[0] - Now()
		if d > 0 {
			delay = int(d * float64(sampleRate))
		}
	}
	inner := newRecipeAwareVoice(id, bpm, sampleRate)
	if inner == nil {
		return
	}
	mix.Schedule(id, &scaledVoice{v: inner, gain: vol}, delay)
}

// PlayParams schedules an instrument with volume, pitch (in semitones), and
// duration multiplier. Pitch and duration are applied via naive resampling
// where playback rate r = 2^(pitch/12) / max(dur, eps). This couples pitch and
// duration (time-stretch not implemented), but allows practical control.
func PlayParams(id string, vol, pitch, dur float64, when ...float64) {
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
	_ = ctx.Resume()
	delay := 0
	if len(when) > 0 {
		d := when[0] - Now()
		if d > 0 {
			delay = int(d * float64(sampleRate))
		}
	}
	// For pitch-aware melodic recipes: re-render at the node pitch so the
	// filter formant stays at an absolute Hz instead of being slid by
	// resampling. The playback rate then carries only the duration factor.
	// For all other instruments (drums, FM, etc.) the old path is unchanged.
	recipeID := RecipeForInstrument(id)
	pitchAware := pitchAwareRecipe(recipeID)
	var v Voice
	if pitchAware {
		v = newRecipeAwareVoicePitched(id, bpm, sampleRate, pitch)
	} else {
		v = newRecipeAwareVoice(id, bpm, sampleRate)
	}
	if v == nil {
		return
	}
	// Compute playback rate from semitones and requested duration multiplier.
	// r > 1 speeds up (higher pitch, shorter time). r < 1 slows down.
	if dur <= 0 {
		dur = 1
	}
	// For pitch-aware voices the buffer is already at-pitch, so pitch factor
	// is not applied — only duration scaling is needed.
	// For non-pitch-aware voices: rate = 2^(semitones/12) / dur (unchanged).
	var rate float64
	if pitchAware {
		rate = 1.0 / dur
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
	if cv, ok := v.(*cVoice); ok && len(cv.buf) > 0 {
		rv.buf = make([]float64, 0, len(cv.buf)+2)
	}
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
		// Longer snare to capture body + ring.
		return (60.0 / float64(b)) * 1.0
	case "kick":
		// Shorter kick for tight, punchy body.
		return (60.0 / float64(b)) * 0.5
	case "hihat":
		// Slightly longer closed hat to match the updated synth tail.
		return (60.0 / float64(b)) * 0.25
	case "tom":
		return (60.0 / float64(b)) * 0.5
	case "clap":
		return (60.0 / float64(b)) * 0.25
	case "cowbell":
		return (60.0 / float64(b)) * 0.5
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
