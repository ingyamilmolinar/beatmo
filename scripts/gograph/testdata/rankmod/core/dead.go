package core

// deadFunc is reached by nothing, not even tests.
func deadFunc(x int) int {
	if x > 0 {
		return x * 2
	}
	return 0
}
