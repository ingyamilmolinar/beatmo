//go:build linux && !test && !js

package audio

import "syscall"

// The render's own cost whatever else runs (`make test-real` runs every package at once). The caller locks its
// goroutine to the thread.
func renderCPUSeconds() float64 {
	const rusageThread = 1 // RUSAGE_THREAD; the syscall package does not name it
	var ru syscall.Rusage
	if syscall.Getrusage(rusageThread, &ru) != nil {
		return 0
	}
	return float64(ru.Utime.Nano()+ru.Stime.Nano()) / 1e9
}
