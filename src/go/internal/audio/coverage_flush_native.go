//go:build !test && !js

package audio

/*
// beatmo_gcov_flush lives in libdrums.a (src/c/gcov_flush.c). In the
// coverage-instrumented build produced by scripts/coverage-c.sh it calls
// __gcov_dump() to write the .gcda counter files; in the normal build it is
// a no-op. The explicit call is required because the Go runtime exits via
// exit_group without running C atexit handlers, so libgcov's exit-time
// flush never fires in a cgo binary. A weak in-line __gcov_dump reference
// would NOT work here: weak references don't pull the defining member out
// of libgcov.a, which is why the strong reference is isolated behind
// -DBEATMO_GCOV in gcov_flush.c.
extern void beatmo_gcov_flush(void);
*/
import "C"

// The Go runtime never runs C exit handlers, so TestMain flushes the gcov counters (make coverage-c).
func FlushCCoverage() {
	C.beatmo_gcov_flush()
}
