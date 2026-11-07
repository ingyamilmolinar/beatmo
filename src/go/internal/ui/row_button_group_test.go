//go:build test

package ui

import (
	"image"
	"testing"
)

func makeTestButton(label string, r image.Rectangle, count *int) *Button {
	b := NewButton(label, ButtonStyle{}, func() { *count++ })
	b.SetRect(r)
	return b
}

func TestRowButtonGroup_AllNil(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	g := RowButtonGroup{} // all fields nil
	if g.HandleInput(50, 50, true) {
		t.Fatal("expected false when all buttons are nil")
	}
}

func TestRowButtonGroup_SingleMatch(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	var calls int
	g := RowButtonGroup{
		Mute: makeTestButton("mute", image.Rect(10, 10, 60, 30), &calls),
	}

	if !g.HandleInput(35, 20, true) {
		t.Fatal("expected true when click is inside Mute button")
	}
	if calls != 1 {
		t.Fatalf("expected callback called once, got %d", calls)
	}
}

// Mute before Solo.
func TestRowButtonGroup_PriorityOrder(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	var muteCalls, soloCalls int
	overlap := image.Rect(10, 10, 60, 30)
	g := RowButtonGroup{
		Mute: makeTestButton("mute", overlap, &muteCalls),
		Solo: makeTestButton("solo", overlap, &soloCalls),
	}

	if !g.HandleInput(35, 20, true) {
		t.Fatal("expected true")
	}
	if muteCalls != 1 {
		t.Fatalf("expected Mute callback called once, got %d", muteCalls)
	}
	if soloCalls != 0 {
		t.Fatalf("expected Solo callback not called, got %d", soloCalls)
	}
}

func TestRowButtonGroup_NonMatchingPosition(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	var calls int
	g := RowButtonGroup{
		Mute:   makeTestButton("mute", image.Rect(10, 10, 60, 30), &calls),
		Solo:   makeTestButton("solo", image.Rect(70, 10, 120, 30), &calls),
		Delete: makeTestButton("del", image.Rect(130, 10, 180, 30), &calls),
	}

	// Click far outside all button rects.
	if g.HandleInput(500, 500, true) {
		t.Fatal("expected false when click is outside all buttons")
	}
	if calls != 0 {
		t.Fatalf("expected no callbacks, got %d", calls)
	}
}

func TestRowButtonGroup_MixedNilAndNonNil(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	var deleteCalls int
	g := RowButtonGroup{
		Mute:   nil, // skipped
		Solo:   nil, // skipped
		FX:     nil, // skipped
		Origin: nil, // skipped
		Delete: makeTestButton("del", image.Rect(10, 10, 60, 30), &deleteCalls),
		// Edit, Save, Menu, Label all nil
	}

	if !g.HandleInput(35, 20, true) {
		t.Fatal("expected true when click is inside Delete button")
	}
	if deleteCalls != 1 {
		t.Fatalf("expected Delete callback called once, got %d", deleteCalls)
	}
}
