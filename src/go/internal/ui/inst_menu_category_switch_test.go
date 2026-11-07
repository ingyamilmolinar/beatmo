//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// The geometry change on a mode switch must not read as a click outside.
func TestInstrumentMenuCategorySwitchDoesNotClose(t *testing.T) {
	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	comp := NewInstrumentMenuComponent()
	closeCalled := false

	comp.SetProps(InstrumentMenuProps{
		Categories: []audio.Category{audio.CatKick, audio.CatSnare, audio.CatCymbal},
		Instruments: []InstrumentOption{
			{ID: roster.Kick, Label: "Kick 1", Category: audio.CatKick},
			{ID: roster.KickDeep, Label: "Kick 2", Category: audio.CatKick},
			{ID: roster.Snare, Label: "Snare 1", Category: audio.CatSnare},
			{ID: roster.SnareLofi, Label: "Snare 2", Category: audio.CatSnare},
			{ID: roster.Crash, Label: "Cymbal 1", Category: audio.CatCymbal},
		},
		AnchorRect:      image.Rect(100, 50, 200, 70),
		VertBounds:      image.Rect(0, 0, 400, 600),
		RowHeight:       24,
		ForceCategories: true,
		OnClose:         func() { closeCalled = true },
	})
	comp.Open()

	// Clear suppress flag so we can test the behavior
	suppressClicksUntilRelease = false

	if comp.Mode() != InstMenuModeCategories {
		t.Fatalf("expected categories mode, got %s", comp.Mode())
	}

	// Find a non-active category button (Snares)
	var targetBtn *Button
	for _, btn := range comp.CategoryBtns() {
		if btn.Text == "Snare" {
			targetBtn = btn
			break
		}
	}
	if targetBtn == nil {
		t.Fatal("could not find Snares category button")
	}

	rect := targetBtn.Rect()
	clickX := rect.Min.X + 5
	clickY := rect.Min.Y + 5

	// A deferred tap: the button fires on release, not here.
	comp.HandleInput(clickX, clickY, true)

	// Menu should still be open (deferred tap hasn't fired yet)
	if !comp.IsOpen() {
		t.Fatal("menu incorrectly closed after pressing category")
	}
	if closeCalled {
		t.Fatal("OnClose should not be called on press")
	}

	// Simulate mouse release — deferred tap fires the category button
	suppressClicksUntilRelease = false
	comp.HandleInput(clickX, clickY, false)

	// After release, category button fires and switches to instruments mode
	if !comp.IsOpen() {
		t.Fatal("menu incorrectly closed after releasing on category")
	}
	if closeCalled {
		t.Fatal("OnClose should not be called when switching categories")
	}
	if comp.Mode() != InstMenuModeInstruments {
		t.Fatalf("expected instruments mode after category click, got %s", comp.Mode())
	}
}

func TestInstrumentMenuBackButtonDoesNotClose(t *testing.T) {
	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	comp := NewInstrumentMenuComponent()
	closeCalled := false

	comp.SetProps(InstrumentMenuProps{
		Categories: []audio.Category{audio.CatKick, audio.CatSnare},
		Instruments: []InstrumentOption{
			{ID: roster.Kick, Label: "Kick 1", Category: audio.CatKick},
			{ID: roster.Snare, Label: "Snare 1", Category: audio.CatSnare},
		},
		AnchorRect:      image.Rect(100, 50, 200, 70),
		VertBounds:      image.Rect(0, 0, 400, 600),
		RowHeight:       24,
		ForceCategories: false, // Start in instruments mode
		OnClose:         func() { closeCalled = true },
	})
	comp.Open()
	suppressClicksUntilRelease = false

	if comp.Mode() != InstMenuModeInstruments {
		t.Fatalf("expected instruments mode, got %s", comp.Mode())
	}

	backBtn := comp.BackBtn()
	if backBtn == nil {
		t.Fatal("back button should exist when categories are available")
	}

	rect := backBtn.Rect()
	clickX := rect.Min.X + 5
	clickY := rect.Min.Y + 5

	// Simulate mouse press on back button
	comp.HandleInput(clickX, clickY, true)

	// Should switch to categories mode
	if !comp.IsOpen() {
		t.Fatal("menu incorrectly closed after clicking back button")
	}
	if closeCalled {
		t.Fatal("OnClose should not be called when clicking back")
	}
	if comp.Mode() != InstMenuModeCategories {
		t.Fatalf("expected categories mode after back click, got %s", comp.Mode())
	}

	// Simulate continued press
	comp.HandleInput(clickX, clickY, true)
	if !comp.IsOpen() {
		t.Fatal("menu closed on continued press after back button")
	}

	// Simulate release
	suppressClicksUntilRelease = false
	comp.HandleInput(clickX, clickY, false)
	if !comp.IsOpen() {
		t.Fatal("menu closed after release")
	}
}

func TestInstrumentMenuMultipleCategorySwitches(t *testing.T) {
	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	comp := NewInstrumentMenuComponent()
	closeCalled := false

	comp.SetProps(InstrumentMenuProps{
		Categories: []audio.Category{audio.CatKick, audio.CatSnare, audio.CatCymbal},
		Instruments: []InstrumentOption{
			{ID: roster.Kick, Label: "Kick 1", Category: audio.CatKick},
			{ID: roster.Snare, Label: "Snare 1", Category: audio.CatSnare},
			{ID: roster.Crash, Label: "Cymbal 1", Category: audio.CatCymbal},
		},
		AnchorRect:      image.Rect(100, 50, 200, 70),
		VertBounds:      image.Rect(0, 0, 400, 600),
		RowHeight:       24,
		ForceCategories: true,
		OnClose:         func() { closeCalled = true },
	})
	comp.Open()
	suppressClicksUntilRelease = false

	// Click "Kicks" category
	var kicksBtn *Button
	for _, btn := range comp.CategoryBtns() {
		if btn.Text == "Kick" {
			kicksBtn = btn
			break
		}
	}
	if kicksBtn == nil {
		t.Fatal("could not find Kicks category button")
	}

	rect := kicksBtn.Rect()
	comp.HandleInput(rect.Min.X+5, rect.Min.Y+5, true)
	suppressClicksUntilRelease = false
	comp.HandleInput(rect.Min.X+5, rect.Min.Y+5, false)

	if !comp.IsOpen() {
		t.Fatal("menu closed after first category switch")
	}
	if comp.ActiveCategory() != audio.CatKick {
		t.Fatalf("expected Kicks category, got %s", comp.ActiveCategory())
	}

	// The deferred category tap set SuppressClicksUntilMouseUp; clear it before the next click.
	suppressClicksUntilRelease = false
	backBtn := comp.BackBtn()
	if backBtn == nil {
		t.Fatal("back button should exist")
	}
	rect = backBtn.Rect()
	comp.HandleInput(rect.Min.X+5, rect.Min.Y+5, true)
	suppressClicksUntilRelease = false
	comp.HandleInput(rect.Min.X+5, rect.Min.Y+5, false)

	if !comp.IsOpen() {
		t.Fatal("menu closed after going back")
	}
	if comp.Mode() != InstMenuModeCategories {
		t.Fatalf("expected categories mode, got %s", comp.Mode())
	}

	// Click "Snares" category
	var snaresBtn *Button
	for _, btn := range comp.CategoryBtns() {
		if btn.Text == "Snare" {
			snaresBtn = btn
			break
		}
	}
	if snaresBtn == nil {
		t.Fatal("could not find Snares category button")
	}

	rect = snaresBtn.Rect()
	comp.HandleInput(rect.Min.X+5, rect.Min.Y+5, true)
	suppressClicksUntilRelease = false
	comp.HandleInput(rect.Min.X+5, rect.Min.Y+5, false)

	if !comp.IsOpen() {
		t.Fatal("menu closed after second category switch")
	}
	if comp.ActiveCategory() != audio.CatSnare {
		t.Fatalf("expected Snares category, got %s", comp.ActiveCategory())
	}

	suppressClicksUntilRelease = false
	backBtn = comp.BackBtn()
	rect = backBtn.Rect()
	comp.HandleInput(rect.Min.X+5, rect.Min.Y+5, true)
	suppressClicksUntilRelease = false
	comp.HandleInput(rect.Min.X+5, rect.Min.Y+5, false)

	if !comp.IsOpen() {
		t.Fatal("menu closed after second back")
	}

	var cymbalsBtn *Button
	for _, btn := range comp.CategoryBtns() {
		if btn.Text == "Cymbal" {
			cymbalsBtn = btn
			break
		}
	}
	if cymbalsBtn == nil {
		t.Fatal("could not find Cymbals category button")
	}

	rect = cymbalsBtn.Rect()
	comp.HandleInput(rect.Min.X+5, rect.Min.Y+5, true)
	suppressClicksUntilRelease = false
	comp.HandleInput(rect.Min.X+5, rect.Min.Y+5, false)

	if !comp.IsOpen() {
		t.Fatal("menu closed after third category switch")
	}
	if comp.ActiveCategory() != audio.CatCymbal {
		t.Fatalf("expected Cymbals category, got %s", comp.ActiveCategory())
	}
	if closeCalled {
		t.Fatal("OnClose should never have been called during category switching")
	}
}
