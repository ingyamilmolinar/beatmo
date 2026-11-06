//go:build !js

package core

// platInit is the non-js stub of a build-tag pair: dead on linux, but its js
// sibling is live, so it is a variant, never a deletion candidate.
func platInit() {}
