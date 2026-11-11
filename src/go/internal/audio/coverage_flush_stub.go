//go:build test || js

package audio

// FlushCCoverage is a no-op outside the native CGo build — there is no C
// object code (and therefore no gcov state) to flush. See
// coverage_flush_native.go for the real implementation.
func FlushCCoverage() {}
