package ui

import "testing"

func TestToSubRoundingNegative(t *testing.T) {
    g := New(testLogger)
    g.Layout(640, 480)
    max := g.grid.MaxDiv()
    if ToSub(g.grid, -1) != -max {
        t.Fatalf("ToSub(-1)=%d want %d", ToSub(g.grid, -1), -max)
    }
    if ToSub(g.grid, -1.5) != -max*3/2 {
        t.Fatalf("ToSub(-1.5)=%d want %d", ToSub(g.grid, -1.5), -max*3/2)
    }
}

