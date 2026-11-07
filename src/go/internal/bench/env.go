// Package bench holds the record-bench harness's pure helpers (env parsing, hooks-config decoding, action dispatch), which
// have real failure modes while cmd/beatmo itself boots Ebiten + audio and cannot be unit tested.
package bench

import (
	"os"
	"strconv"
)

// def for unset, empty, non-numeric or non-positive: "0" counts as unset.
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
