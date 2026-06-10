//go:build test

package audio

// PlayBatch is a test stub; UI tests override playback via Game.SetPlayFunc.
// It still stamps the trigger clock per request (like the real desktop/WASM
// batch paths and the stub's own Play*) so trigger-driven UI stays testable.
func PlayBatch(reqs []BatchParam) {
	for i := range reqs {
		RecordVoiceTrigger(reqs[i].ID)
	}
}
