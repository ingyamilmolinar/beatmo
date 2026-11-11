//go:build test

package core

// TaggedHelper is test code living in a production directory.
func TaggedHelper() int { return taggedOnly() }
