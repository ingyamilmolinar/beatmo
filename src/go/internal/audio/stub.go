//go:build test

package audio

// Play is a stub used during tests to avoid initializing audio devices.
func Play(id string, when ...float64) {}

// Now returns 0 during tests.
func Now() float64 { return 0 }

// Reset is a stub used during tests.
func Reset() {}

var SetBPMFunc = func(int) {}

func SetBPM(bpm int) { SetBPMFunc(bpm) }
