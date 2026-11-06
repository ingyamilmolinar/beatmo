package core

// helperForTest is only used by core_test.go.
func helperForTest() int { return helper() + 1 }

// taggedOnly is only called from tagged_helper.go, a non-test file compiled
// solely under the test build tag.
func taggedOnly() int { return 3 }
