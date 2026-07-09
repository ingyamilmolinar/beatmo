//go:build js && !test

package ui

import "syscall/js"

var (
	miFnRegister        js.Value
	miFnRegisterTrigger js.Value
	miFnClear           js.Value
	miFnActive          js.Value
	miFnAnyActive       js.Value
	miFnPollResult      js.Value
	miFnGetValue        js.Value
	miFnClose           js.Value
	miFnCloseAll        js.Value
	miInitialized       bool
)

// mobileInputInit caches JS function references for the mobile native input system.
// Called once from softKeyboardInit().
func mobileInputInit() {
	g := js.Global()
	miFnRegister = g.Get("_mobileInputRegister")
	miFnRegisterTrigger = g.Get("_mobileInputRegisterTrigger")
	miFnClear = g.Get("_mobileInputClear")
	miFnActive = g.Get("_mobileInputActive")
	miFnAnyActive = g.Get("_mobileInputAnyActive")
	miFnPollResult = g.Get("_mobileInputPollResult")
	miFnGetValue = g.Get("_mobileInputGetValue")
	miFnClose = g.Get("_mobileInputClose")
	miFnCloseAll = g.Get("_mobileInputCloseAll")
	miInitialized = miFnRegister.Truthy() && miFnActive.Truthy() && miFnPollResult.Truthy()
}

func mobileInputRegister(id string, x, y, w, h int, text string, maxLen int, inputMode string) {
	if !miInitialized {
		return
	}
	miFnRegister.Invoke(id, x, y, w, h, text, maxLen, inputMode)
}

func mobileInputRegisterTrigger(id string, trigX, trigY, trigW, trigH, inputX, inputY, inputW, inputH int, text string, maxLen int, inputMode string) {
	if !miInitialized {
		return
	}
	miFnRegisterTrigger.Invoke(id, trigX, trigY, trigW, trigH, inputX, inputY, inputW, inputH, text, maxLen, inputMode)
}

func mobileInputClear() {
	if !miInitialized {
		return
	}
	miFnClear.Invoke()
}

func mobileInputActive(id string) bool {
	if !miInitialized {
		return false
	}
	return miFnActive.Invoke(id).Bool()
}

// lastPointerWasTouch reports whether the MOST RECENT pointer interaction was a
// touch or pen (vs a mouse). This — NOT device touch-CAPABILITY — is the correct
// signal for "will the native <input> overlay be created?": the overlay is built
// only inside a `touchend` handler (see index.html), so it appears iff the
// opening gesture was a touch. Using capability (navigator.maxTouchPoints) was
// the bug: a touch-capable laptop / DevTools device-mode driven with a MOUSE
// reports capability=true yet fires no touchend, so the native input never
// appeared and the mobile editor sat inert ("click, see cursor, no key works").
// Backed by a pointerdown listener in index.html; read live (cheap bool call).
func lastPointerWasTouch() bool {
	fn := js.Global().Get("_lastPointerWasTouch")
	if !fn.Truthy() {
		return false
	}
	return fn.Invoke().Bool()
}

func mobileInputAnyActive() bool {
	if !miInitialized {
		return false
	}
	return miFnAnyActive.Invoke().Bool()
}

// mobileInputPollResult returns the result of a completed native input.
// Returns (value, committed, ok). ok is false if no result is available.
func mobileInputPollResult(id string) (string, bool, bool) {
	if !miInitialized {
		return "", false, false
	}
	r := miFnPollResult.Invoke(id)
	if r.IsNull() || r.IsUndefined() {
		return "", false, false
	}
	value := r.Get("value").String()
	committed := r.Get("committed").Bool()
	return value, committed, true
}

// mobileInputGetValue reads the current value without consuming the result.
func mobileInputGetValue(id string) (string, bool) {
	if !miInitialized {
		return "", false
	}
	r := miFnGetValue.Invoke(id)
	if r.IsNull() || r.IsUndefined() {
		return "", false
	}
	return r.String(), true
}

func mobileInputClose(id string) {
	if !miInitialized {
		return
	}
	miFnClose.Invoke(id)
}

func mobileInputCloseAll() {
	if !miInitialized {
		return
	}
	miFnCloseAll.Invoke()
}
