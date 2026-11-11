package core

import "testing"

func TestHelperForTest(t *testing.T) {
	if helperForTest() != 2 {
		t.Fatal("bad")
	}
}
