//go:build test

package audio

import "sync/atomic"

var playBatchHookForTest atomic.Pointer[func([]BatchParam)]

// The lane oracle tests compare the legacy sequencer's full dispatch against the lane's notes.
func SetPlayBatchHookForTest(fn func([]BatchParam)) (restore func()) {
	prev := playBatchHookForTest.Load()
	if fn == nil {
		playBatchHookForTest.Store(nil)
	} else {
		playBatchHookForTest.Store(&fn)
	}
	return func() { playBatchHookForTest.Store(prev) }
}

// Stamps the trigger clock per request, as the real batch paths do, so trigger-driven UI stays testable.
func PlayBatch(reqs []BatchParam) {
	for i := range reqs {
		if reqs[i].VoiceID == 0 { // lane voices: stamped by the UI lane on the beat
			RecordVoiceTrigger(reqs[i].ID)
		}
	}
	if h := playBatchHookForTest.Load(); h != nil {
		(*h)(append([]BatchParam(nil), reqs...))
	}
}
