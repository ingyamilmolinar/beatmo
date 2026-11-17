// Package bench houses small, testable helpers extracted from the
// cmd/beatmo bench/record-bench harness. The cmd/beatmo entry point
// remains untestable as a whole (it boots Ebiten + audio), but these
// pure helpers — env parsing, hooks-config decoding, action dispatch —
// have real failure modes (bad JSON, unknown action, negative env
// values) that deserve unit coverage.
package bench

import (
	"os"
	"strconv"
)

// EnvInt parses the named environment variable as a positive integer.
// Returns def if the variable is unset, empty, non-numeric, or
// non-positive. "0" is therefore treated as unset.
//
// Used by cmd/beatmo to gate optional runtime profiling
// (BEATMO_BLOCK_PROFILE, BEATMO_MUTEX_PROFILE).
func EnvInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}
