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

// stretchClock times the work between two yields on the thread's CPU clock:
// the wall clock also counts every preemption, and a parallel `go test ./...`
// preempts a whole Warm, failing the stretch bound on correct code.
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
