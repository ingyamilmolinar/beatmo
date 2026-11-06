package core

// SupportForTests is test-support API used by ANOTHER package's tests: it
// cannot move into a _test.go file (those are invisible across packages).
func SupportForTests() int { return helper() * 10 }
