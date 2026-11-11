//go:build test

package core

import "testing"

func TestTagged(t *testing.T) {
	if TaggedHelper() != 3 {
		t.Fatal("bad")
	}
}
