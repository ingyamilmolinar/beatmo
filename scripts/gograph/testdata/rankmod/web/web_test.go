package web

import (
	"testing"

	"example.com/rankmod/core"
)

func TestUsesCoreSupport(t *testing.T) {
	if core.SupportForTests() != 10 {
		t.Fatal("bad")
	}
}
