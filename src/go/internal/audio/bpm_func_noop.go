//go:build !test

package audio

// SetBPMFuncForTest is a no-op in production builds.
func SetBPMFuncForTest(fn func(int)) {
	_ = fn
}
