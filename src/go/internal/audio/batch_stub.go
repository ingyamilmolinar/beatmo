//go:build test

package audio

import "sync/atomic"

var playBatchHookForTest atomic.Pointer[func([]BatchParam)]

// SetPlayBatchHookForTest installs a capture of every PlayBatch call (test
// build only): the audio-lane oracle tests compare the legacy sequencer's
// full dispatch (id, vol, pitch, row, abs) against the lane's notes.
func SetPlayBatchHookForTest(fn func([]BatchParam)) (restore func()) {
	prev := playBatchHookForTest.Load()
	if fn == nil {
		playBatchHookForTest.Store(nil)
	} else {
		playBatchHookForTest.Store(&fn)
	}
	return func() { playBatchHookForTest.Store(prev) }
}

// PlayBatch is a test stub; UI tests override playback via Game.SetPlayFunc.
// It still stamps the trigger clock per request (like the real desktop/WASM
// batch paths and the stub's own Play*) so trigger-driven UI stays testable.
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
