//go:build !js || test

package ui

import "image"

// testMobileInputActive is a per-ID active state for test injection.
var testMobileInputActive map[string]bool

// testMobileInputResult is a per-ID pending result for test injection.
var testMobileInputResult map[string]*struct {
	Value     string
	Committed bool
}

// testMobileInputValue is a per-ID current value for test injection (for search).
var testMobileInputValue map[string]string

// testMobileInputRegistered tracks direct registrations for test assertions.
var testMobileInputRegistered map[string]bool

// testMobileInputTriggerRegistered tracks trigger registrations for test assertions.
var testMobileInputTriggerRegistered map[string]bool

// testMobileInputRect records the registered rect for direct registrations.
var testMobileInputRect map[string]image.Rectangle

// testMobileInputTriggerRect records the registered TRIGGER rect (the tap
// target that activates the native input), so tests can assert a trigger lands
// on the intended control and not an adjacent one.
var testMobileInputTriggerRect map[string]image.Rectangle

//nolint:unused // cross-build-tag stub matching mobile_input_wasm.go
func mobileInputInit() {}

func mobileInputRegister(id string, x, y, w, h int, text string, maxLen int, inputMode string) {
	if testMobileInputRegistered != nil {
		testMobileInputRegistered[id] = true
	}
	if testMobileInputRect != nil {
		testMobileInputRect[id] = image.Rect(x, y, x+w, y+h)
	}
}

func mobileInputRegisterTrigger(id string, trigX, trigY, trigW, trigH, inputX, inputY, inputW, inputH int, text string, maxLen int, inputMode string) {
	if testMobileInputTriggerRegistered != nil {
		testMobileInputTriggerRegistered[id] = true
	}
	if testMobileInputTriggerRect != nil {
		testMobileInputTriggerRect[id] = image.Rect(trigX, trigY, trigX+trigW, trigY+trigH)
	}
}

func mobileInputClear() {
	for k := range testMobileInputRegistered {
		delete(testMobileInputRegistered, k)
	}
	for k := range testMobileInputTriggerRegistered {
		delete(testMobileInputTriggerRegistered, k)
	}
	for k := range testMobileInputRect {
		delete(testMobileInputRect, k)
	}
	for k := range testMobileInputTriggerRect {
		delete(testMobileInputTriggerRect, k)
	}
}

func mobileInputActive(id string) bool {
	if testMobileInputActive != nil {
		return testMobileInputActive[id]
	}
	return false
}

// lastPointerWasTouchForTest overrides lastPointerWasTouch() under the stub.
// Defaults to true so existing mobile tests (which force the mobile profile and
// drive the native <input> path) keep exercising that path without change. A
// test simulating "mobile layout driven with a MOUSE" (touch-capable device or
// narrow desktop, pointer=mouse) sets this to false.
var lastPointerWasTouchForTest = true

// lastPointerWasTouch reports whether the most recent pointer interaction was a
// touch/pen. See the WASM implementation in mobile_input_wasm.go.
func lastPointerWasTouch() bool { return lastPointerWasTouchForTest }

//nolint:unused // cross-build-tag stub matching mobile_input_wasm.go
func mobileInputAnyActive() bool {
	for _, v := range testMobileInputActive {
		if v {
			return true
		}
	}
	return false
}

func mobileInputPollResult(id string) (string, bool, bool) {
	if testMobileInputResult != nil {
		if r, ok := testMobileInputResult[id]; ok && r != nil {
			v, c := r.Value, r.Committed
			delete(testMobileInputResult, id)
			return v, c, true
		}
	}
	return "", false, false
}

//nolint:unused // cross-build-tag stub matching mobile_input_wasm.go
func mobileInputGetValue(id string) (string, bool) {
	if testMobileInputValue != nil {
		if v, ok := testMobileInputValue[id]; ok {
			return v, true
		}
	}
	return "", false
}

func mobileInputClose(id string) {
	if testMobileInputActive != nil {
		delete(testMobileInputActive, id)
	}
	if testMobileInputResult != nil {
		delete(testMobileInputResult, id)
	}
	if testMobileInputValue != nil {
		delete(testMobileInputValue, id)
	}
}

//nolint:unused // cross-build-tag stub matching mobile_input_wasm.go
func mobileInputCloseAll() {
	testMobileInputActive = nil
	testMobileInputResult = nil
	testMobileInputValue = nil
}
