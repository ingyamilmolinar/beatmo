//go:build js

package foo

// Plat has one variant per platform; only the configured GOOS's file counts.
func Plat() int { return 1 }
