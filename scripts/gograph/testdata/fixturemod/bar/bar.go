package bar

import "example.com/fixture/foo"

// Run calls both a free function and a method on an imported type.
func Run() int {
	t := foo.Thing{N: 3}
	return foo.Helper(t.Process())
}
