package ui

import (
	"reflect"
	"testing"
)

func TestPackCardRowsWrapsGreedily(t *testing.T) {
	got := packCardRows([]int{100, 200, 300, 150, 500}, 600, 10)
	want := [][]int{{0, 1}, {2, 3}, {4}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("packCardRows = %v, want %v", got, want)
	}
	// A card wider than the row gets a row of its own and never an empty row.
	got = packCardRows([]int{700, 50}, 600, 10)
	want = [][]int{{0}, {1}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("packCardRows(oversize) = %v, want %v", got, want)
	}
	if rows := packCardRows(nil, 600, 10); len(rows) != 0 {
		t.Fatalf("packCardRows(nil) = %v, want no rows", rows)
	}
}

func TestJustifyRowFillsTheRowExactly(t *testing.T) {
	ws := justifyRow([]int{100, 300}, 610, 10)
	if ws[0]+ws[1]+10 != 610 {
		t.Fatalf("justifyRow = %v: does not fill 610 with a 10 px gap", ws)
	}
	if ws[1] < 2*ws[0] {
		t.Fatalf("justifyRow = %v: the extra must follow each card's width", ws)
	}
}
