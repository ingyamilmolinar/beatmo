//go:build test

package audio

// SetBPMFuncForTest overrides the test-only BPM hook. In production builds,
// this is a no-op to avoid leaking test-only symbols.
func SetBPMFuncForTest(fn func(int)) {
	if fn == nil {
		fn = func(int) {}
	}
	bpmFunc.Store(&fn)
}
