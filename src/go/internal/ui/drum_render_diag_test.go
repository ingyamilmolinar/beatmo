//go:build test

package ui

import "testing"

func TestDetectSlimBars(t *testing.T) {
	cases := []struct {
		name   string
		widths []int
		wantOK bool
		wantAt int
	}{
		{"uniform clean", []int{9, 9, 9, 9, 9}, false, -1},
		{"uniform with rounding", []int{8, 9, 8, 9, 8}, false, -1},
		{"single slim flanked", []int{9, 9, 1, 9, 9}, true, 2},
		{"two-px slim flanked", []int{8, 2, 9}, true, 1},
		{"slim at edge not flanked", []int{1, 9, 9}, false, -1},
		{"trailing slim not flanked", []int{9, 9, 1}, false, -1},
		{"empty", []int{}, false, -1},
		{"all slim (narrow grid, not a bug)", []int{2, 2, 2, 2}, false, -1},
		{"slim between narrow not flagged", []int{3, 1, 3}, false, -1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			at, ok := detectSlimBars(c.widths)
			if ok != c.wantOK || (ok && at != c.wantAt) {
				t.Fatalf("detectSlimBars(%v) = (%d,%v); want (%d,%v)", c.widths, at, ok, c.wantAt, c.wantOK)
			}
		})
	}
}
