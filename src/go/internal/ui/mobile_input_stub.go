//go:build !js || test

package ui

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

//nolint:unused // cross-build-tag stub matching mobile_input_wasm.go
func mobileInputInit() {}

func mobileInputRegister(id string, x, y, w, h int, text string, maxLen int, inputMode string) {
	if testMobileInputRegistered != nil {
		testMobileInputRegistered[id] = true
	}
}

func mobileInputRegisterTrigger(id string, trigX, trigY, trigW, trigH, inputX, inputY, inputW, inputH int, text string, maxLen int, inputMode string) {
	if testMobileInputTriggerRegistered != nil {
		testMobileInputTriggerRegistered[id] = true
	}
}

func mobileInputClear() {
	for k := range testMobileInputRegistered {
		delete(testMobileInputRegistered, k)
	}
	for k := range testMobileInputTriggerRegistered {
		delete(testMobileInputTriggerRegistered, k)
	}
}

func mobileInputActive(id string) bool {
	if testMobileInputActive != nil {
		return testMobileInputActive[id]
	}
	return false
}

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
