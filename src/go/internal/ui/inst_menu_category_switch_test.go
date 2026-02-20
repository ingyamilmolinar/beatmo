//go:build test

package ui

import (
	"image"
	"testing"
)

// TestInstrumentMenuCategorySwitchDoesNotClose verifies that clicking a
// category button switches to instruments mode without incorrectly closing
// the menu due to geometry changes.
func TestInstrumentMenuCategorySwitchDoesNotClose(t *testing.T) {
	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	comp := NewInstrumentMenuComponent()
	closeCalled := false

	comp.SetProps(InstrumentMenuProps{
		Categories: []string{"Kicks", "Snares", "Cymbals"},
		Instruments: []InstrumentOption{
			{ID: "kick1", Label: "Kick 1", Category: "Kicks"},
			{ID: "kick2", Label: "Kick 2", Category: "Kicks"},
			{ID: "snare1", Label: "Snare 1", Category: "Snares"},
			{ID: "snare2", Label: "Snare 2", Category: "Snares"},
			{ID: "cymbal1", Label: "Cymbal 1", Category: "Cymbals"},
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
		if btn.Text == "Snares" {
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

	// Simulate mouse press on the category button.
	// With deferred tap, the button won't fire until release.
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

// TestInstrumentMenuBackButtonDoesNotClose verifies that clicking the Back
// button returns to categories mode without closing the menu.
func TestInstrumentMenuBackButtonDoesNotClose(t *testing.T) {
	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	comp := NewInstrumentMenuComponent()
	closeCalled := false

	comp.SetProps(InstrumentMenuProps{
		Categories: []string{"Kicks", "Snares"},
		Instruments: []InstrumentOption{
			{ID: "kick1", Label: "Kick 1", Category: "Kicks"},
			{ID: "snare1", Label: "Snare 1", Category: "Snares"},
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

// TestInstrumentMenuMultipleCategorySwitches verifies that switching between
// multiple categories in sequence works correctly without closing the menu.
func TestInstrumentMenuMultipleCategorySwitches(t *testing.T) {
	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	comp := NewInstrumentMenuComponent()
	closeCalled := false

	comp.SetProps(InstrumentMenuProps{
		Categories: []string{"Kicks", "Snares", "Cymbals"},
		Instruments: []InstrumentOption{
			{ID: "kick1", Label: "Kick 1", Category: "Kicks"},
			{ID: "snare1", Label: "Snare 1", Category: "Snares"},
			{ID: "cymbal1", Label: "Cymbal 1", Category: "Cymbals"},
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
		if btn.Text == "Kicks" {
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
	if comp.ActiveCategory() != "Kicks" {
		t.Fatalf("expected Kicks category, got %s", comp.ActiveCategory())
	}

	// Click back button to return to categories.
	// Clear suppress first — the deferred category tap set it via SuppressClicksUntilMouseUp.
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
		if btn.Text == "Snares" {
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
	if comp.ActiveCategory() != "Snares" {
		t.Fatalf("expected Snares category, got %s", comp.ActiveCategory())
	}

	// Go back again and switch to Cymbals.
	// Clear suppress first.
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
		if btn.Text == "Cymbals" {
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
	if comp.ActiveCategory() != "Cymbals" {
		t.Fatalf("expected Cymbals category, got %s", comp.ActiveCategory())
	}
	if closeCalled {
		t.Fatal("OnClose should never have been called during category switching")
	}
}
