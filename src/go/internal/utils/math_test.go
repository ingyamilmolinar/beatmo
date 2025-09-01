package utils

import "testing"

func TestGCD(t *testing.T) {
    if GCD(8,12) != 4 {
        t.Fatalf("expected 4")
    }
    if GCD(-6,9) != 3 {
        t.Fatalf("expected 3")
    }
}

func TestClamp01(t *testing.T) {
    if Clamp01(-1) != 0 || Clamp01(2) != 1 || Clamp01(0.5) != 0.5 {
        t.Fatalf("clamp failed")
    }
}
