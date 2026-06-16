//go:build js && !test

package ui

import "syscall/js"

var (
	kbProxyFocusFn   js.Value
	kbProxyBlurFn    js.Value
	kbProxyDrainFn   js.Value
	kbRegisterRectFn js.Value
	kbClearRectsFn   js.Value
	kbEscFn          js.Func
	kbArrowLeftFn    js.Func
	kbArrowRightFn   js.Func
	kbInitialized    bool
)

// softKeyboardInit caches JS function references for the keyboard proxy.
// Called once during initJS().
func softKeyboardInit() {
	g := js.Global()
	kbProxyFocusFn = g.Get("_kbProxyFocus")
	kbProxyBlurFn = g.Get("_kbProxyBlur")
	kbProxyDrainFn = g.Get("_kbProxyDrain")
	kbRegisterRectFn = g.Get("_kbRegisterFocusRect")
	kbClearRectsFn = g.Get("_kbClearFocusRects")
	kbInitialized = kbProxyFocusFn.Truthy() && kbProxyBlurFn.Truthy() && kbProxyDrainFn.Truthy()
	// Bridge the proxy's Escape key to the universal Esc handler. The hidden
	// proxy input holds keyboard focus while a text field is active, so the
	// canvas never sees Esc — the proxy's keydown handler calls this.
	kbEscFn = js.FuncOf(func(js.Value, []js.Value) any {
		SignalSoftKeyboardEscape()
		return nil
	})
	g.Set("_kbProxyEscapeGo", kbEscFn)
	// Bridge the proxy's caret-navigation keys (Arrow Left/Right) too: the proxy
	// holds keyboard focus, so the canvas never sees them and caret movement was
	// dead on WASM. The proxy's keydown handler forwards them via these funcs.
	kbArrowLeftFn = js.FuncOf(func(js.Value, []js.Value) any {
		SignalSoftKeyboardArrowLeft()
		return nil
	})
	g.Set("_kbProxyArrowLeftGo", kbArrowLeftFn)
	kbArrowRightFn = js.FuncOf(func(js.Value, []js.Value) any {
		SignalSoftKeyboardArrowRight()
		return nil
	})
	g.Set("_kbProxyArrowRightGo", kbArrowRightFn)
	mobileInputInit()
}

// softKeyboardShow focuses the hidden proxy input to trigger the mobile soft keyboard.
func softKeyboardShow(inputmode string) {
	if !kbInitialized {
		return
	}
	kbProxyFocusFn.Invoke(inputmode)
}

// softKeyboardHide blurs the proxy input to dismiss the soft keyboard.
func softKeyboardHide() {
	if !kbInitialized {
		return
	}
	kbProxyBlurFn.Invoke()
}

// softKeyboardDrainChars drains accumulated characters from the proxy input.
// Returns runes including '\b' for backspace and '\n' for Enter.
func softKeyboardDrainChars() []rune {
	if !kbInitialized {
		return nil
	}
	arr := kbProxyDrainFn.Invoke()
	if !arr.Truthy() {
		return nil
	}
	length := arr.Length()
	if length == 0 {
		return nil
	}
	runes := make([]rune, 0, length)
	for i := 0; i < length; i++ {
		s := arr.Index(i).String()
		for _, r := range s {
			runes = append(runes, r)
		}
	}
	return runes
}

// softKeyboardActive reports whether the proxy input is currently focused.
func softKeyboardActive() bool {
	if !kbInitialized {
		return false
	}
	doc := js.Global().Get("document")
	active := doc.Get("activeElement")
	if !active.Truthy() {
		return false
	}
	return active.Get("id").String() == "beatmo-kb-proxy"
}

// softKeyboardRegisterRect registers a canvas region as focusable for the
// gesture-based soft keyboard system. On touchend inside this rect, the JS
// layer will call proxy.focus() synchronously within the trusted gesture.
func softKeyboardRegisterRect(id string, x, y, w, h int, inputmode string) {
	if !kbRegisterRectFn.Truthy() {
		return
	}
	kbRegisterRectFn.Invoke(id, x, y, w, h, inputmode)
}

// softKeyboardClearRects removes all registered focusable rects.
func softKeyboardClearRects() {
	if !kbClearRectsFn.Truthy() {
		return
	}
	kbClearRectsFn.Invoke()
}
