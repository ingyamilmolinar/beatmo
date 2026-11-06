//go:build !js

package core

// Plat has one variant per platform.
func Plat() int {
	if helper() > 0 {
		return 2
	}
	return 0
}
