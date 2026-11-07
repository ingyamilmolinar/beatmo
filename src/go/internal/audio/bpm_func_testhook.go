//go:build test

package audio

// A no-op in production builds, so no test-only symbol leaks.
func SetBPMFuncForTest(fn func(int)) {
	if fn == nil {
		fn = func(int) {}
	}
	bpmFunc.Store(&fn)
}
