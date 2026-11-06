//go:build linux

package soundview

import (
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

// clockThreadCPUTimeID is Linux's CLOCK_THREAD_CPUTIME_ID.
const clockThreadCPUTimeID = 3

// stretchClock returns a clock of the work the calling goroutine does: the
// CPU time of its OS thread, which it is pinned to until release is called.
// A stretch between two yields is how much WORK runs without handing the
// thread back; the wall clock also counts every time the OS scheduler ran
// something else, and under `go test ./...` (8 packages at once, the ui suite
// among them) a ~15 ms Warm was preempted on every one of five runs, so the
// best of five read 10-15 ms for code that does ~1 ms between yields.
func stretchClock() (now func() time.Duration, release func()) {
	runtime.LockOSThread()
	return func() time.Duration {
		var ts syscall.Timespec
		if _, _, errno := syscall.RawSyscall(syscall.SYS_CLOCK_GETTIME, clockThreadCPUTimeID, uintptr(unsafe.Pointer(&ts)), 0); errno != 0 {
			panic("clock_gettime(CLOCK_THREAD_CPUTIME_ID): " + errno.Error())
		}
		return time.Duration(ts.Nano())
	}, runtime.UnlockOSThread
}
