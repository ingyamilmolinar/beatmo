package utils

import "testing"

func TestGCD(t *testing.T) {
	if GCD(8, 12) != 4 {
		t.Fatalf("expected 4")
	}
	if GCD(-6, 9) != 3 {
		t.Fatalf("expected 3")
	}
	if GCD(0, 0) != 0 {
		t.Fatalf("expected 0 for gcd(0,0)")
	}
	if GCD(0, 5) != 5 {
		t.Fatalf("expected 5 for gcd(0,5)")
	}
}

func TestClamp01(t *testing.T) {
	if Clamp01(-1) != 0 || Clamp01(2) != 1 || Clamp01(0.5) != 0.5 || Clamp01(1) != 1 || Clamp01(0) != 0 {
		t.Fatalf("clamp failed")
	}
}

func TestAbs(t *testing.T) {
	if Abs(-3) != 3 || Abs(0) != 0 || Abs(4) != 4 {
		t.Fatalf("abs failed")
	}
}

func TestCalculateIntermediateGridPoints(t *testing.T) {
	pts := CalculateIntermediateGridPoints(0, 0, 0, 3)
	if len(pts) != 2 || pts[0].Y != 1 || pts[1].Y != 2 {
		t.Fatalf("unexpected vertical points: %v", pts)
	}
	pts = CalculateIntermediateGridPoints(0, 0, 3, 0)
	if len(pts) != 2 || pts[0].X != 1 || pts[1].X != 2 {
		t.Fatalf("unexpected horizontal points: %v", pts)
	}
	pts = CalculateIntermediateGridPoints(0, 0, 1, 1)
	if len(pts) != 0 {
		t.Fatalf("expected no intermediate points for diagonal, got %v", pts)
	}
	pts = CalculateIntermediateGridPoints(0, 0, 0, 1)
	if len(pts) != 0 {
		t.Fatalf("expected no intermediate points for adjacent nodes, got %v", pts)
	}
}
