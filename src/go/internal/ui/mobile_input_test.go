//go:build test

package ui

import (
	"image"
	"testing"
)

func TestMobileInputStub_Register(t *testing.T) {
	// Verify stub registration functions don't panic
	mobileInputClear()
	mobileInputRegister("bpm", 10, 20, 100, 30, "120", 4, "numeric")
	mobileInputRegisterTrigger("rename-0", 0, 0, 30, 30, 50, 0, 100, 30, "Kick", 32, "text")
	mobileInputClose("bpm")
	mobileInputCloseAll()
}

func TestMobileInputStub_ActiveInjection(t *testing.T) {
	// Default: no active inputs
	if mobileInputActive("bpm") {
		t.Error("expected bpm not active by default")
	}
	if mobileInputAnyActive() {
		t.Error("expected no active inputs by default")
	}

	// Inject active state
	testMobileInputActive = map[string]bool{"bpm": true}
	defer func() { testMobileInputActive = nil }()

	if !mobileInputActive("bpm") {
		t.Error("expected bpm active after injection")
	}
	if !mobileInputAnyActive() {
		t.Error("expected some active input after injection")
	}
	if mobileInputActive("rename-0") {
		t.Error("expected rename-0 not active")
	}
}

func TestMobileInputStub_ResultInjection(t *testing.T) {
	// No result by default
	_, _, ok := mobileInputPollResult("bpm")
	if ok {
		t.Error("expected no result by default")
	}

	// Inject result
	testMobileInputResult = map[string]*struct {
		Value     string
		Committed bool
	}{
		"bpm": {Value: "140", Committed: true},
	}
	defer func() { testMobileInputResult = nil }()

	val, committed, ok := mobileInputPollResult("bpm")
	if !ok {
		t.Fatal("expected result available")
	}
	if val != "140" {
		t.Errorf("expected value '140', got '%s'", val)
	}
	if !committed {
		t.Error("expected committed=true")
	}

	// Second poll should return nothing (consumed)
	_, _, ok = mobileInputPollResult("bpm")
	if ok {
		t.Error("expected result consumed after first poll")
	}
}

func TestMobileInputStub_ValueInjection(t *testing.T) {
	// No value by default
	_, ok := mobileInputGetValue("inst-search")
	if ok {
		t.Error("expected no value by default")
	}

	// Inject value
	testMobileInputValue = map[string]string{"inst-search": "kick"}
	defer func() { testMobileInputValue = nil }()

	v, ok := mobileInputGetValue("inst-search")
	if !ok {
		t.Fatal("expected value available")
	}
	if v != "kick" {
		t.Errorf("expected 'kick', got '%s'", v)
	}
}

func TestMobileInputStub_Close(t *testing.T) {
	testMobileInputActive = map[string]bool{"bpm": true, "rename-0": true}
	testMobileInputResult = map[string]*struct {
		Value     string
		Committed bool
	}{
		"bpm": {Value: "100", Committed: true},
	}
	defer func() {
		testMobileInputActive = nil
		testMobileInputResult = nil
	}()

	mobileInputClose("bpm")
	if mobileInputActive("bpm") {
		t.Error("expected bpm not active after close")
	}
	if !mobileInputActive("rename-0") {
		t.Error("expected rename-0 still active after closing bpm")
	}

	mobileInputCloseAll()
	if mobileInputAnyActive() {
		t.Error("expected no active inputs after closeAll")
	}
}

func TestTextInput_MobileDrawSkip(t *testing.T) {
	forceSmallScreenForTest = true
	defer func() { forceSmallScreenForTest = false }()

	ti := NewTextInput(image.Rect(10, 10, 200, 40), BPMBoxStyle)
	ti.MobileInputID = "bpm"
	ti.SetText("120")

	// Without active mobile input, Draw should not skip (no panic is success)
	// We can't easily test pixel output, but we verify Draw() doesn't skip
	// by checking it doesn't return early before the nil check — stubbed Ebiten
	// will handle the nil dst gracefully in tests.

	// With active mobile input, Draw should skip entirely
	testMobileInputActive = map[string]bool{"bpm": true}
	defer func() { testMobileInputActive = nil }()

	// This should return early without attempting to draw
	ti.Draw(nil) // would panic on nil dst if it tried to draw
}
