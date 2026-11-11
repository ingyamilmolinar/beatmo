//go:build !js

package foo

// Plat has one variant per platform; only the configured GOOS's file counts.
func Plat() int {
	if Helper(1) > 0 {
		return 2
	}
	return 0
}
