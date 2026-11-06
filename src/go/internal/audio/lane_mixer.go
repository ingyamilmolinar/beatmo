//go:build !test && !js

package audio

import (
	"sync/atomic"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
)

// laneNoCV marks a lane voice with no underlying cVoice (test sine voices):
// its trigger is still reported (hook) but carries no buffer.
var laneNoCV = &cVoice{}

type laneTrigger struct {
	id channel.ID
	cv *cVoice
}

// laneTriggerCh hands "lane voice started rendering" events from the audio
// thread (processBlock, under m.mu) to laneTriggerLoop, which does the
// analyzer / scope work (an ~80 KB conversion per trigger) OFF the audio
// thread. A full channel drops the meter/scope notification, never audio.
var laneTriggerCh = make(chan laneTrigger, 256)

var laneTriggerHook atomic.Pointer[func(string)]

// setLaneTriggerHookForTest observes delivered lane triggers (tests only).
func setLaneTriggerHookForTest(fn func(string)) (restore func()) {
	prev := laneTriggerHook.Load()
	laneTriggerHook.Store(&fn)
	return func() { laneTriggerHook.Store(prev) }
}

// Started at package init so goleak's baseline (VerifyTestMain with
// IgnoreCurrent) already contains it.
func init() { go laneTriggerLoop() }

func laneTriggerLoop() {
	for t := range laneTriggerCh {
		cv := t.cv
		if cv == laneNoCV {
			cv = nil
		}
		notifyVoiceTrigger(t.id, cv)
		if h := laneTriggerHook.Load(); h != nil {
			(*h)(t.id.String())
		}
	}
}

// laneStarted runs on the audio thread at a lane voice's first rendered
// block (caller holds m.mu): non-blocking handoff of its trigger.
func (m *mixer) laneStarted(vs *voiceState) {
	cv := vs.laneCV
	if cv == nil {
		return
	}
	vs.laneCV = nil
	select {
	case laneTriggerCh <- laneTrigger{id: vs.id, cv: cv}:
	default:
	}
}

// cancelVoices silently removes lane voices that have not produced a sample
// yet. Lock order matches Read: pendingMu → mu.
func (m *mixer) cancelVoices(ids []uint64) (started []uint64) {
	if len(ids) == 0 {
		return nil
	}
	set := make(map[uint64]struct{}, len(ids))
	for _, id := range ids {
		if id != 0 {
			set[id] = struct{}{}
		}
	}
	return m.removeLaneVoices(func(vs *voiceState) bool { _, ok := set[vs.vid]; return ok })
}

// cancelAllLaneFuture removes every lane voice (vid != 0) not yet started.
func (m *mixer) cancelAllLaneFuture() {
	m.removeLaneVoices(func(vs *voiceState) bool { return vs.vid != 0 })
}

// removeLaneVoices drops pending voices matching sel and active voices
// matching sel whose start is still in the future (a removed voice never
// rendered a sample, so removal is silent — unlike RequestStop, which plays
// a faded ~20 ms ghost when the start arrives).
// It returns the selected voices it REFUSED because they have started
// rendering (the caller must not replace those — see audioSink.Replace).
func (m *mixer) removeLaneVoices(sel func(*voiceState) bool) (started []uint64) {
	m.pendingMu.Lock()
	keep := m.pendingAdd[:0]
	for _, vs := range m.pendingAdd {
		if vs.vid != 0 && sel(vs) {
			continue
		}
		keep = append(keep, vs)
	}
	for i := len(keep); i < len(m.pendingAdd); i++ {
		m.pendingAdd[i] = nil
	}
	m.pendingAdd = keep
	m.mu.Lock()
	for i := 0; i < len(m.voices); i++ {
		vs := m.voices[i]
		if vs.vid == 0 || !sel(vs) {
			continue
		}
		if vs.start > m.pos {
			m.voices[i] = m.voices[len(m.voices)-1]
			m.voices[len(m.voices)-1] = nil
			m.voices = m.voices[:len(m.voices)-1]
			i--
		} else {
			started = append(started, vs.vid)
		}
	}
	m.mu.Unlock()
	m.pendingMu.Unlock()
	return started
}

// stopLaneAtSample queues a stop applied by processBlock when the block
// containing atSample renders.
func (m *mixer) stopLaneAtSample(inst channel.ID, atSample int) {
	m.mu.Lock()
	m.laneStops = append(m.laneStops, laneStopEvent{inst: inst, atSample: atSample})
	m.mu.Unlock()
}

// applyLaneStops (caller holds mu): fade lane voices of each due event's
// instrument that started before its sample; spare later ones.
func (m *mixer) applyLaneStops(blockEnd int) {
	if len(m.laneStops) == 0 {
		return
	}
	keep := m.laneStops[:0]
	for _, ev := range m.laneStops {
		if ev.atSample >= blockEnd {
			keep = append(keep, ev)
			continue
		}
		for _, vs := range m.voices {
			if vs.vid != 0 && vs.id == ev.inst && vs.start < ev.atSample {
				if ap, ok := vs.v.(*antiPopVoice); ok {
					ap.RequestStopAt(ev.atSample - vs.start)
				}
			}
		}
	}
	m.laneStops = keep
}
